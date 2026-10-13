package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

func TestHealthz(t *testing.T) {
	response := httptest.NewRecorder()
	newAuthHandler(nil).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got, want := response.Body.String(), `{"status":"ok"}`; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestHealthzRejectsOtherMethods(t *testing.T) {
	response := httptest.NewRecorder()
	newAuthHandler(nil).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/healthz", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

func TestLoadAuthConfig(t *testing.T) {
	for _, tc := range []struct {
		name    string
		values  map[string]string
		wantErr bool
	}{
		{name: "complete", values: map[string]string{"OIDC_ISSUER": "https://issuer.example", "OIDC_AUDIENCE": "api"}},
		{name: "missing issuer", values: map[string]string{"OIDC_AUDIENCE": "api"}, wantErr: true},
		{name: "missing audience", values: map[string]string{"OIDC_ISSUER": "https://issuer.example"}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadAuthConfig(func(k string) string { return tc.values[k] })
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestOIDCAuthenticationAndRBAC(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	issuer := ""
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/keys" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{"kty": "RSA", "kid": "test-key", "use": "sig", "alg": "RS256", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes())}}})
	}))
	defer jwks.Close()
	var issuerServer *httptest.Server
	issuerServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"issuer": issuer, "jwks_uri": jwks.URL + "/keys"})
	}))
	defer issuerServer.Close()
	issuer = issuerServer.URL
	v, err := newOIDCVerifier(context.Background(), authConfig{issuer: issuer, audience: "api"})
	if err != nil {
		t.Fatal(err)
	}
	handler := newAuthHandler(v)
	valid := signTestToken(t, key, issuer, map[string]any{"roles": []string{"user", "admin"}})
	user := signTestToken(t, key, issuer, map[string]any{"roles": []string{"user"}})
	badKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, path, token string
		want              int
	}{
		{"health public", "/healthz", "", http.StatusOK}, {"missing token", "/api/me", "", http.StatusUnauthorized},
		{"valid me", "/api/me", valid, http.StatusOK}, {"valid admin", "/api/admin", valid, http.StatusOK},
		{"role denied", "/api/admin", user, http.StatusForbidden},
		{"bad signature", "/api/me", signTestToken(t, badKey, issuer, map[string]any{"roles": []string{"user"}}), http.StatusUnauthorized},
		{"wrong issuer", "/api/me", signTestToken(t, key, "http://wrong.example", map[string]any{"roles": []string{"user"}}), http.StatusUnauthorized},
		{"wrong audience", "/api/me", signWrongAudience(t, key, issuer), http.StatusUnauthorized},
		{"expired", "/api/me", signTestToken(t, key, issuer, map[string]any{"roles": []string{"user"}, "exp": time.Now().Add(-time.Minute).Unix()}), http.StatusUnauthorized},
		{"not yet valid", "/api/me", signTestToken(t, key, issuer, map[string]any{"roles": []string{"user"}, "nbf": time.Now().Add(time.Hour).Unix()}), http.StatusUnauthorized},
		{"malformed", "/api/me", "not-a-token", http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if response.Code != tc.want {
				t.Fatalf("status = %d, want %d", response.Code, tc.want)
			}
		})
	}
}

func signWrongAudience(t *testing.T, key *rsa.PrivateKey, issuer string) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithHeader("kid", "test-key"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := jwt.Signed(signer).Claims(map[string]any{"iss": issuer, "aud": "other", "sub": "user-1", "exp": time.Now().Add(time.Hour).Unix()}).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func signTestToken(t *testing.T, key *rsa.PrivateKey, issuer string, claims map[string]any) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "test-key"))
	if err != nil {
		t.Fatal(err)
	}
	claims["iss"] = issuer
	claims["aud"] = "api"
	claims["sub"] = "user-1"
	claims["iat"] = time.Now().Unix()
	if _, ok := claims["exp"]; !ok {
		claims["exp"] = time.Now().Add(time.Hour).Unix()
	}
	raw, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
