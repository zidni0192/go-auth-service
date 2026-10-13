# Go Auth Service

A local-first Go service for OIDC/JWT authentication and role-based access control. No Docker or external service is required for local development.

## Run

Requires Go 1.27 or newer.

```sh
go run ./cmd/server
```

The service listens on `:8080`; set `ADDR` to change the address. `GET /healthz` reports service health.

## Verify

```sh
go test -race ./...
go vet ./...
```

Authentication behavior is being developed in issue-based pull requests. Do not use this starter as an authentication provider until token verification and authorization are implemented and tested.
