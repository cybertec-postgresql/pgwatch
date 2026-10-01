# AGENTS.md

pgwatch v6: Go (1.26) PostgreSQL monitoring agent + React/Vite WebUI. Module `github.com/cybertec-postgresql/pgwatch/v6`. Single binary `./cmd/pgwatch`.

## Build prerequisites (easy to miss)

- `internal/webserver/webserver.go` does `//go:embed build`. `internal/webserver/build/` is gitignored and produced by the WebUI build. Without it, Go build/test/lint all fail. Build it once: `cd internal/webui && yarn install --network-timeout 100000 && yarn build` (or `task ui`).
- `api/pb/*.pb.go` are gitignored generated files. Regenerate after editing `api/pb/pgwatch.proto`: `go generate ./api/pb/` or `task proto` (needs `protoc` + `protoc-gen-go`, `protoc-gen-go-grpc`; `task tools` installs the Go plugins).
- Task runner is `Taskfile.yml` (go-task). `task --list` shows everything. `task check` = lint + test (the pre-PR gate).
- Run locally: `go run ./cmd/pgwatch/ <flags>` (see `--help`). Logs go to stdout.

## Test

- Tests use testcontainers (Postgres `postgres:19beta2-alpine`, etcd, see `internal/testutil/types.go`). A running Docker daemon is required. There are no build tags: `*_integration_test.go` files run with the normal suite.
- CI command: `go test -failfast -v -timeout=300s -p 1 ./cmd/... ./internal/...`. Keep `-p 1`: packages share containers/ports.
- One package: `go test -failfast -p 1 -timeout=300s ./internal/reaper`. One test: add `-run TestName`.
- `internal/sinks/rpc_test.go` has a `TestMain` that starts gRPC test servers via `testutil.SetupRPCServers()`.
- Mocks: `pgxmock/v5` for DB code, helpers in `internal/testutil`.
- Coverage: add `-coverprofile=coverage.out`, view with `go tool cover -html=coverage.out`.

## Architecture

- Concepts: **source** (monitored DB: postgres/pgbouncer/pgpool/patroni/prometheus), **metric** (SQL query definition), **sink** (json/grpc/postgres/timescale/prometheus), **reaper** (schedules and gathers metrics from sources, writes them to sinks). See `docs/concept/components.md`.
- `internal/cmdopts`: CLI options and subcommands (`pgwatch metric print-init`, `pgwatch config upgrade`, ...). `go-flags` based.
- `internal/metrics/metrics.yaml` is embedded and is the built-in default metric/preset definitions. Changes there ship in the binary.
- SQL is embedded (`//go:embed`) from `internal/sinks/sql/*.sql` and `internal/metrics/postgres_schema.sql`.
- Schema migrations use `pgx-migrator`. Lists are in `internal/sinks/postgres.go` (sink DB) and `internal/metrics/postgres_schema.go` (config DB). Append new migrations at the end; never edit or reorder existing ones. Names start with a zero-padded number (e.g. `"01409 ..."`).
- Layout: `api/` (protobuf), `cmd/` (entry point), `internal/` (all Go code), `internal/webui/` (React UI), `docker/` (compose files), `grafana/` (dashboards), `docs/` (mkdocs site).
- `spec/` holds design docs and task templates for larger features.
