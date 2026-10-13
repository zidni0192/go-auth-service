package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

type authConfig struct {
	issuer   string
	audience string
}

func loadAuthConfig(getenv func(string) string) (authConfig, error) {
	config := authConfig{issuer: strings.TrimSpace(getenv("OIDC_ISSUER")), audience: strings.TrimSpace(getenv("OIDC_AUDIENCE"))}
	if config.issuer == "" || config.audience == "" {
		return authConfig{}, errors.New("OIDC_ISSUER and OIDC_AUDIENCE must both be configured")
	}
	return config, nil
}

type verifier interface {
	Verify(context.Context, string) (*oidc.IDToken, error)
}

type verifiedClaims struct {
	Subject string   `json:"sub"`
	Roles   []string `json:"roles"`
}

type contextKey string

func claimsFromContext(ctx context.Context) verifiedClaims {
	claims, _ := ctx.Value(contextKey("verified-claims")).(verifiedClaims)
	return claims
}

func bearerAuth(v verifier, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "Bearer "
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, prefix) || strings.TrimSpace(strings.TrimPrefix(header, prefix)) == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
		idToken, err := v.Verify(r.Context(), token)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var claims verifiedClaims
		if err := idToken.Claims(&claims); err != nil || claims.Subject == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey("verified-claims"), claims)))
	})
}

func requireRole(role string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, got := range claimsFromContext(r.Context()).Roles {
			if got == role {
				next.ServeHTTP(w, r)
				return
			}
		}
		http.Error(w, "forbidden", http.StatusForbidden)
	})
}

func newAuthHandler(v verifier) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("GET /api/me", bearerAuth(v, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(claimsFromContext(r.Context()))
	})))
	mux.Handle("GET /api/admin", bearerAuth(v, requireRole("admin", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))))
	return mux
}

func newOIDCVerifier(ctx context.Context, config authConfig) (verifier, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	ctx = oidc.ClientContext(ctx, client)
	provider, err := oidc.NewProvider(ctx, config.issuer)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC issuer: %w", err)
	}
	return provider.Verifier(&oidc.Config{ClientID: config.audience}), nil
}
