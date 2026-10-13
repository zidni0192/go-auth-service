# Go Auth Service

A local-first Go service for OIDC/JWT authentication and role-based access control. No Docker or external service is required for local development.

## Run

Requires Go 1.27 or newer.

Set `OIDC_ISSUER` to the exact issuer URL and `OIDC_AUDIENCE` to the API audience before starting; both are required. The service discovers the issuer metadata and signing keys at startup using the maintained `github.com/coreos/go-oidc/v3/oidc` verifier. Discovery and key requests use a five-second timeout, and verified keys are cached by the verifier. No token is logged.

```sh
export OIDC_ISSUER=https://your-issuer.example
export OIDC_AUDIENCE=your-api
go run ./cmd/server
```

The service listens on `:8080`; set `ADDR` to change the address. `GET /healthz` is public, `GET /api/me` requires a valid bearer token, and `GET /api/admin` additionally requires the `admin` role claim. No Docker or live provider is needed to run the deterministic tests.

## Verify

```sh
go test -race ./...
go vet ./...
```

Authentication behavior is being developed in issue-based pull requests. Do not use this starter as an authentication provider until token verification and authorization are implemented and tested.
