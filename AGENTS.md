# Project instructions

- Keep local setup Docker-free and avoid real credentials or production identity providers in tests.
- Validate JWT signatures and claims against explicitly configured trusted issuers; reject unsafe algorithms and malformed claims.
- Add security-focused tests for every authentication or authorization change. Do not claim security guarantees beyond tested behavior.
- Before the first commit, verify local Git identity is `zidni0192 <izedeledby@gmail.com>`. Use feature branches and PRs; never commit directly to `main`.
- Run `go test -race ./...` and `go vet ./...` before PR. GitHub auto-merge is enabled only after required CI checks pass; never bypass branch protection.
