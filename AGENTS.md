# Guppy — Agent Guide

Go client library and CLI for the Storacha network, built for enterprise-scale, resumable uploads with parallel processing. Module: `github.com/fil-forge/guppy` (Go 1.26). Compare with the JS `@storacha/cli`: guppy targets large, long-running, restartable uploads backed by a local database rather than one-shot web workflows.

## Quick Reference

```bash
make build             # Build ./guppy binary (ldflags inject version/commit/date from version.json + git)
make test              # go test ./...
make migration NAME=x  # Create a new Goose migration in pkg/preparation/sqlrepo/migrations
make migrate-up        # Apply migrations to ~/.fil-forge/guppy/preparation.db (goose via `go tool goose`)
make guppy-prod        # Stripped production binary
make guppy-debug       # No-optimization binary with full symbols
make docker-prod       # Multi-arch (amd64+arm64) Docker image
make release           # scripts/release.sh
```

User-facing docs are an mkdocs site in `docs/` (`docs_dir: content`) — see `docs/content/configuration.md` for the full config reference and `docs/content/cli/` for command docs. Keep those in sync when changing flags or config keys.

## Structure

- `main.go` — entry point: OTel SDK setup, signal handling (SIGINT/SIGTERM), pprof server
- `cmd/` — Cobra CLI commands: `account/`, `blob/`, `delegation/`, `gateway/`, `identity/`, `proof/`, `space/`, `unixfs/`, `upload/`, plus top-level `login`, `ls`, `reset`, `retrieve`, `verify`, `whoami`, `version`
- `pkg/client/` — Storacha network client: one file per invocation (`blobadd.go`, `indexadd.go`, `uploadadd.go`, `requestaccess.go`, `claimaccess.go`, `retrieve.go`, …), plus `dagservice/`, `locator/`, `testutil/`
- `pkg/preparation/` — core upload pipeline: scan → DAG → shards → upload
  - `sqlrepo/` — SQL persistence (Goose migrations, WAL checkpointing for SQLite)
  - domain packages: `spaces/`, `uploads/`, `sources/`, `scans/`, `dags/`, `blobs/`, `storacha/`, `check/`, `types/`
- `pkg/tokenstore/` — UCAN token persistence (delegations, invocations, receipts): `MemStore`, `FsStore`; proof selection via `ProofChain`
- `pkg/verification/` — content verification: indexer queries + retrieval checks
- `pkg/config/` — Viper config structs split by domain (`repo.go`, `network.go`, `gateway.go`, `identity.go`, `upload.go`) with validation
- `pkg/presets/` — named network presets and env-var overrides
- `pkg/bus/` — event bus (asaskevich/EventBus) for component communication; event types in `pkg/bus/events/`
- `pkg/dagfs/`, `pkg/didmailto/`, `pkg/build/` — DAG filesystem view, did:mailto helpers, build metadata
- `internal/` — `cmdutil/` (CLI helpers, `HandledCliError`), `telemetry/`, `testutil/`, `fakefs/`, `largeupload/`
- `test/` — end-to-end shell scripts (`doupload`, `baduploads`, `gatewayretrieve`); `scripts/` — kubo gateway + TLS proxy helpers for retrieval testing
- `examples/` — `byoidentity/`, `loginflow/` library-usage examples

## Key Patterns

- **Functional options**: `client.New(issuer, serviceID, serviceURL, options...)` with `client.WithTokenStore(...)`, `client.WithReceiptsClient(...)`, `client.WithUCANClientOptions(...)`, `client.WithRetrievalOptions(...)`.
- **Repository pattern**: each `pkg/preparation` domain package defines its own `Repo` interface; `sqlrepo.Repo` implements all of them.
- **API/Repo separation**: business logic in `API` structs, data access behind `Repo` interfaces.
- **UCAN capabilities**: `ucantone` for capability-based auth; delegations/invocations/receipts persisted in the token store.

## Key Dependencies

- `github.com/fil-forge/ucantone` — UCAN implementation (client, DIDs, delegations)
- `github.com/fil-forge/libforge` — shared capabilities, receipts, retrieval client
- `github.com/fil-forge/indexing-service` — indexer client types
- `github.com/ipfs/boxo`, `go-cid`, `go-ipld-prime`, `go-unixfsnode` — IPFS/IPLD
- `github.com/ipld/go-car` (+ v2) — CAR format
- `modernc.org/sqlite` — pure-Go SQLite (default DB); `lib/pq` + `sqlx` for optional PostgreSQL
- `github.com/pressly/goose/v3` — migrations (run via `go tool goose`)
- `github.com/spf13/cobra` + `viper` — CLI and config
- `github.com/charmbracelet/bubbletea` — TUI

## Config & State

- Data dir: `~/.fil-forge/guppy/` (override with `--data-dir` or `repo.data_dir`). Holds `preparation.db` and token-store state.
- Config file (TOML), first found wins: `--config` path → `~/.config/guppy/config.toml` → `./config.toml`. Env vars use the `GUPPY_` prefix (e.g. `GUPPY_GATEWAY_PORT`). Precedence: flags > env > file.
- Database: SQLite by default; setting `repo.database_url` (postgres URL) switches the sqlrepo to PostgreSQL. `sqlrepo` rewrites `?` placeholders to `$N` for Postgres and skips SQLite-specific WAL checkpointing.
- Networks: presets `forge`, `forge-test`, `hot`, `warm-staging` selected via config `network.name` or `STORACHA_NETWORK`. Individual endpoints can be overridden via `STORACHA_SERVICE_DID`, `STORACHA_SERVICE_URL`, `STORACHA_RECEIPTS_URL`, `STORACHA_INDEXING_SERVICE_DID`, `STORACHA_INDEXING_SERVICE_URL`, `STORACHA_AUTHORIZED_RETRIEVALS` (see `pkg/presets/presets.go`).

## Conventions

- Logging: `var log = logging.Logger("pkg/name")` (ipfs/go-log), structured calls `log.Infow()`, `log.Warnw()`, `log.Debugw()`.
- Tracing: `var tracer = otel.Tracer("pkg/name")`; spans around commands and pipeline stages.
- Errors: wrap with `%w` and context; CLI errors already shown to the user are wrapped in `cmdutil.HandledCliError` (see `internal/cmdutil`) so they aren't printed twice.
- Tests: standard `go test` with interface mocks, `testutil` packages (`pkg/client/testutil`, `internal/testutil`), and `t.TempDir()` for filesystem work.

## Gotchas

- **ucantone migration**: the old `Proofs(CapabilityQuery)` wildcard-matching API and the `WithStore` / `WithPrincipal` / `WithAdditionalProofs` client options were removed. Proof selection now lives in the token store's `ProofChain` (logic exercised in libforge's own tests). Don't reintroduce the old patterns from older examples or docs.
- Migrations live in `pkg/preparation/sqlrepo/migrations` and must work on both SQLite and PostgreSQL; check `sqlrepo/transform.go` and dialect handling before writing raw SQL.
- Multiple upload sources per space are not yet well supported (UI limitations) — noted in the README.
- On Windows, never build inside a OneDrive-synced folder (corrupts Go build artifacts) — see CONTRIBUTING.md.
