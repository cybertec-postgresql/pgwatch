# AGENTS.md

## Build prerequisites (easy to miss)

- `internal/webserver/webserver.go` does `//go:embed build`; `internal/webserver/build/` is gitignored and produced by `task ui`. Without it Go build/test/lint all fail.
- `api/pb/*.pb.go` are gitignored generated files. Regenerate after editing `api/pb/pgwatch.proto` with `task proto` (`task tools` installs the Go plugins; `protoc` is needed too).
- Use the Taskfile: `task --list`. `task check` (lint + test) is the pre-PR gate.

## Test

- `task test`. Tests use testcontainers (image in `internal/testutil/types.go`), so a running Docker daemon is required. No build tags: `*_integration_test.go` run with the normal suite.
- One package or test: `go test -failfast -p 1 -timeout=300s ./internal/reaper -run TestName`. Keep `-p 1`: packages share containers/ports.
- Mocks: `pgxmock/v5` for DB code, helpers in `internal/testutil`.

## Architecture

- Terms: **source** (monitored DB), **metric** (SQL query definition), **sink** (metric storage), **reaper** (gathers metrics from sources into sinks). See `docs/concept/components.md`.
- `internal/metrics/metrics.yaml` is embedded and is the built-in default metric/preset definitions. Changes there ship in the binary.
- Schema migrations (`pgx-migrator`) live in `internal/sinks/postgres.go` (sink DB) and `internal/metrics/postgres_schema.go` (config DB). Append new ones at the end; never edit or reorder existing ones. Names start with a zero-padded number (e.g. `"01409 ..."`).
- `spec/` holds design docs and task templates for larger features.

## Contributing

- Follow `AI_POLICY.md`: every PR, issue or comment assisted by an AI tool must name that tool.
