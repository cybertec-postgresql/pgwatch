---
title: "The web base path is accepted with or without slashes"
version: 1.0
date_created: 2026-09-25
date_updated: 2026-09-25
owner: pgwatch maintainers
status: draft
tags: [webserver, reverse-proxy, configuration]
---

# Introduction

`--web-base-path` (`PW_WEBBASEPATH`) moves the web UI and the REST API under a path, for a
reverse proxy that serves pgwatch at `https://host/pgwatch/`. The server builds its route
prefix as `"/" + WebBasePath + "/"` and trims nothing, so only the spelling `pgwatch` works.
`/pgwatch` registers every route under `//pgwatch/`, and `pgwatch/` under `/pgwatch//`: the
server starts, answers `404` on every path the proxy forwards, and logs nothing that names the
cause. Operators write paths with a leading slash by habit, and most proxy configurations do.

This specification makes the server accept the value in any of the four spellings and use one
normalized form everywhere it reads it. The spelling that works today keeps working.

Code references are against branch `v7`. Line numbers are indicative, not contractual.

---

## 1. Purpose & Scope

**Purpose**: `pgwatch`, `/pgwatch`, `pgwatch/` and `/pgwatch/` give the same routes.

**In scope**: the value of `--web-base-path` / `PW_WEBBASEPATH`; the route prefix built from it
in `pkg/webserver/webserver.go`; the base path handed to the UI template and to the route
extension hook.

**Out of scope**: new options; paths with more than one segment beyond what the current code
already allows; the UI's own handling of the injected base path.

**Audience**: pgwatch maintainers; operators running pgwatch behind a reverse proxy; embedders
that register routes through the route hook.

**Assumptions**: none.

---

## 2. Definitions

| Term | Definition |
|---|---|
| **Base path** | The value of `--web-base-path` after normalization: no leading or trailing slash, empty for the root. |
| **Route prefix** | `/` for an empty base path, otherwise `/<base path>/`. Every route is registered under it. |

---

## 3. Requirements, Constraints & Guidelines

- **REQ-001**: The server trims every leading and trailing `/` from `--web-base-path` once, when
  it reads the options, and uses the trimmed value wherever it uses the base path today: the
  route prefix (`webserver.go:75-78`), the `BasePath` of the UI template (`webserver.go:133`),
  the prefix `handleStatic` strips (`webserver.go:162`), and the base path passed to the route
  extension hook.
- **REQ-002**: A value that is empty after trimming (`/`, `//`) means the root, as an empty value
  does today.
- **REQ-003**: A value with an empty segment inside (`a//b`) or a character outside the URL path
  set is rejected at startup with an error that names the option and the value.
- **CON-001**: `--web-base-path=pgwatch` behaves exactly as today: the same routes, the same
  `BasePath` in `index.html`.
- **GUD-001**: The option description names the accepted forms, for example
  `Base path for web UI and API endpoints ('pgwatch' or '/pgwatch/')`.

---

## 4. Interfaces & Data Contracts

No new option and no change to the REST API. For `--web-base-path` values:

| Value | Route prefix today | Route prefix after this change |
|---|---|---|
| (empty) | `/` | `/` |
| `pgwatch` | `/pgwatch/` | `/pgwatch/` |
| `/pgwatch` | `//pgwatch/` | `/pgwatch/` |
| `pgwatch/` | `/pgwatch//` | `/pgwatch/` |
| `/pgwatch/` | `//pgwatch//` | `/pgwatch/` |
| `/` | `//` | `/` |

---

## 5. Acceptance Criteria

- **AC-001**: A table test over the six values of section 4 asserts the route prefix, and that
  `GET /pgwatch/liveness` answers `200` for each of the four spellings of `pgwatch`.
- **AC-002**: The rendered `index.html` carries the same `BasePath` for the four spellings of
  `pgwatch`.
- **AC-003**: `--web-base-path=a//b` fails at startup with a message naming `--web-base-path`.

---

## 6. Test Automation Strategy

Unit tests in `pkg/webserver` with `httptest`, following the existing webserver tests. No
integration test is needed: the change is string handling before the mux is built.

---

## 7. Rationale & Context

### Why normalize instead of documenting the one working spelling?

The failure is silent: the server starts, every request answers `404`, and nothing in the log
names the option. The help text shows `pgwatch` as an example, but reverse proxy configurations,
Kubernetes ingress paths and most operators write `/pgwatch`. A two-line trim removes the whole
class of mistake at no cost to the spelling that works.

### Why reject `a//b` instead of collapsing it?

An empty segment is almost always a typing mistake in a longer path; collapsing it would hide
the mistake behind a path the proxy does not forward to.

---

## 8. Dependencies & External Integrations

- `pkg/webserver/cmdopts.go:16` (the option), `pkg/webserver/webserver.go:75-78`,
  `:97`, `:133`, `:162`.
- The route extension hook, whose callers receive the route prefix and should see the
  normalized form.
