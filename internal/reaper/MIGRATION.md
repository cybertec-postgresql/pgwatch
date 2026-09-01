# Log parsing: the pglogwatch migration

`internal/reaper` no longer parses PostgreSQL logs. It resolves the server's
GUCs, decides local versus remote, counts severities, and emits the envelope;
[pglogwatch](https://github.com/cybertec-postgresql/pglogwatch) does the
parsing. This file records what changed for anyone reading `logparser.go` and
wondering where the regex went, and what is still outstanding.

## What was replaced

| before | after |
|---|---|
| `logparser_local.go`, a `bufio` loop over `*.csv` | `pglogwatch.FileSet` |
| `logparser_remote.go`, chunked `pg_read_file` | `pglogwatch/pgremote` |
| a 12-group regex over csvlog | `pglogwatch.Parser` |
| csvlog only | csvlog, jsonlog and stderr |
| resumption by line count, skipped by re-reading | resumption by byte offset |
| `pgSeveritiesLocale`, ten languages inline | `pglogwatch.Config.MessagesLang` |

602 lines deleted, and the deletion is the point: two parsers with separate
rotation, splitting and resumption logic were two places for the same bug.

## What deliberately did not change

**The envelope.** `server_log_event_counts` emits the same sixteen keys with
the same names and the same `int64` type. Dashboards select those names out of
the measurement JSON, so the schema is a wire format, not an implementation
detail. Two tests hold it: one pins the key set, and one reads the shipped
Grafana dashboard and requires every key its panels select to be emitted.

**Numbered debug severities are still dropped.** PostgreSQL writes `DEBUG1`
through `DEBUG5`, never a bare `DEBUG`, and `GetMeasurementEnvelope` only reads
the eight names in `pgSeverities` — so the `debug` column has emitted zero on
every real server for years. pglogwatch reports the same numbered severities,
and the obvious adapter folds them into `DEBUG`. That would look like a bug fix
while changing a column with years of zeroes behind it, so the drop is
preserved and `TestNumberedDebugSeveritiesAreDropped` says so.

**Start at end of file.** Both old parsers started at the end of an existing
log. A fresh pgwatch that read from the start would count every severity in
months of retained logs and report the lot as one interval's worth.

**`logging_collector` is still a hard error**, where the `log_destination` one
was removed. They are not alike: `log_destination` only chooses a format, and
all three are readable now; with the collector off there are no log files in
`log_directory` at all.

## What is new

**Concurrency.** The send interval ticks in its own goroutine while parsing
runs in another, so `eventCounts` is guarded by `countsMu`. The old loops were
single-goroutine and needed no lock. This is not gratuitous: a following reader
blocks when the server is quiet, and checking the interval between records —
what the old loops did at EOF — would mean a quiet server stops reporting
entirely.

**Bounded offsets that do not lose the active file.** The old code capped its
map by clearing all of it, which discarded the offset of the file being read at
that moment; that file was then re-seeded to its current end and everything
written since was skipped. Silent, and reachable only on a server with
thousands of rotated logs. Eviction is now least-recently-used, and the active
file is by definition the most recently used.

## Known behaviour worth knowing

**A stderr record is complete only when the next one begins.** `DETAIL`,
`HINT` and `STATEMENT` lines belong to the record above them, so the parser
cannot close a record until it sees the line after it. The most recent stderr
record is therefore always pending, and its counts arrive one record later. On
a live server this is invisible. It does not apply to csvlog or jsonlog, where
a record ends at its newline.

**Offsets do not survive a process restart.** `endSeededOffsets` is in memory,
as `fileOffsets` was before it. A pgwatch restart re-seeds each file to its
current end, so records written while pgwatch was down are not counted.
Unchanged by this migration, and the `OffsetStore` interface is the seam where
a persistent store would go.

## Outstanding

**pglogwatch is not released.** `go.mod` carries two `replace` directives
pointing at a sibling checkout: pglogwatch lives in its own repository and
freezes its API at v1.0, and that release does not exist yet. **This branch
cannot be merged until it does.** When it is tagged, delete the two `replace`
lines and pin the version; nothing else changes.

The suite should ultimately pass against the released module. It has been run
against the working tree instead — `go build ./...`, `go vet ./...`, the full
`go test ./...`, and `go test -race -count=2 ./internal/reaper/` all pass — but
that is not the same thing, and it is not claimed to be.
