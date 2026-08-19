---
title: Diátaxis Documentation Audit
---

# pgwatch Documentation Audit — Diátaxis Framework

**Date:** 2026-08-19
**Scope:** `docs/` tree (40 files across `intro/`, `concept/`, `tutorial/`, `howto/`, `reference/`, `gallery/`, `developer/`, plus `index.md` and `_overrides/`).
**Operating mode:** `audit` (classify every file, flag violations, propose structural improvements).
**Reference:** [Diátaxis skill](../.agents/skills/diataxis/references/quadrants.md), mkdocs nav at `mkdocs.yml`.

---

## Executive Summary

| Quadrant | Count | Quality | Notes |
|---|---|---|---|
| Explanation | 9 | Mixed — most are good, a few drift into how-to | Concept + intro + parts of `concept/long_term_installations.md` |
| How-to | 7 | Good shape; some are skeletal | All under `howto/` — coherent |
| Reference | 6 | Mostly strong; two are mixed-mode | `cli_env.md`, `rest.md`, `sinks_options.md` are excellent |
| Tutorial | 3 | All three are mixed with reference content | Heavy "explainer" digressions break the learning flow |
| Mixed-mode violations | 8 | — | Listed below; require restructuring |
| Misfiled in nav | 1 | — | `howto/sizing_recommendations.md` listed under "Concepts" (fixed in S2) |
| Dropped temp doc | 1 | — | `developer/reaper-batch-consolidation.md` confirmed temp and removed |

**Top three findings:**

1. **Concept/Tutorial/Reference quadrants bleed into each other.** Tutorial files contain long explanation sections; reference files (`metric_definitions.md`, `advanced_features.md`) embed how-to steps; explanation files (`concept/security.md`) end with sample shell commands.
2. **Folder names roughly match quadrants, but content does not.** `tutorial/` contains mixed tutorial/reference; `concept/` is mostly good explanation but two files are how-to. The folder ↔ quadrant mapping is therefore brittle and not a reliable navigation cue.
3. **`reference/env_variables.md` is a near-empty stub that overlaps with `reference/cli_env.md`.** The Gatherer daemon section simply links out; only Docker-specific variables are listed. Either merge or expand.

---

## 1. Inventory & Quadrant Classification

Legend — **T** Tutorial · **H** How-to · **R** Reference · **E** Explanation · **M** Mixed (multi-quadrant, violation).

| File | Folder | Quadrant | Confidence | One-line summary |
|---|---|---|---|---|
| `index.md` | root | E + nav links | high | Landing page — mix of explanation + nav pointers (acceptable per skill) |
| `intro/project_background.md` | intro/ | **E** | high | Project history, design goals, blog links |
| `intro/features.md` | intro/ | **E** | high | Discursive feature inventory; no params/specs |
| `concept/components.md` | concept/ | **E** | high | Architecture overview, component-by-component description |
| `concept/installation_options.md` | concept/ | **E** | high | Config-DB vs YAML trade-offs |
| `concept/long_term_installations.md` | concept/ | **E** | high | Long-term operational strategy and conventions |
| `concept/security.md` | concept/ | **M (E + H)** | medium | Mostly conceptual; ends with a sample `docker run` block — partial how-to |
| `concept/web_ui.md` | concept/ | **E** | high | What the Web UI is, defaults, security toggle notes |
| `concept/kubernetes.md` | concept/ | **M (E + H)** | medium | Brief conceptual note + bare `helm install` commands; no context for the values file |
| `tutorial/docker_installation.md` | tutorial/ | **M (T + R)** | medium | Sequential install steps mixed with image catalog + port list |
| `tutorial/custom_installation.md` | tutorial/ | **M (T + H)** | medium | Numbered setup steps; introduces branches (Postgres vs YAML config store) |
| `tutorial/preparing_databases.md` | tutorial/ | **M (T + R)** | medium | Step-by-step prep + long discursive "PL/Python helpers" / source-types reference sections |
| `tutorial/upgrading.md` | tutorial/ | **M (T + R)** | medium | Step-by-step upgrade paths with embedded rationale and link-outs |
| `howto/config_db_bootstrap.md` | howto/ | **H** | high | Goal-oriented; numbered step sequence with terminal output |
| `howto/metrics_db_bootstrap.md` | howto/ | **H** | high | Same shape as config_db_bootstrap |
| `howto/migrate_v2_to_v3.md` | howto/ | **H** | high | Migration recipe with SQL migrations and inserts |
| `howto/dashboarding_alerting.md` | howto/ | **M (E + H)** | medium | Predominantly discursive ("we don't want to be too opinionated here"); few concrete steps |
| `howto/using_managed_services.md` | howto/ | **M (R + E)** | medium | Per-provider reference list + contextual caveats |
| `howto/monitor_prometheus_exporter.md` | howto/ | **M (H + R)** | medium | Quick-start YAML snippet + reference tables for presets/connection string |
| `howto/implement_grpc_server.md` | howto/ | **H** | high | Outbound link to contrib tutorial — short but correctly framed |
| `howto/reverse_proxy.md` | howto/ | **H** | high | Apache + Nginx configs + verification steps |
| `howto/sizing_recommendations.md` | howto/ | **R** | medium | Rules-of-thumb table; misfiled under "Concepts" in `mkdocs.yml` |
| `reference/cli_env.md` | reference/ | **R** | high | Canonical CLI flag + env-var spec |
| `reference/sinks_options.md` | reference/ | **R** | high | URI format per sink |
| `reference/rest.md` | reference/ | **R** | high | Endpoint catalogue with request/response shapes |
| `reference/env_variables.md` | reference/ | **R (stub)** | high | Thin; only Docker + Grafana vars — gatherer section defers to `cli_env.md` |
| `reference/metric_definitions.md` | reference/ | **M (E + R + H)** | medium | "What is a metric" (E) + YAML schema (R) + Web-UI add-metric steps (H) |
| `reference/advanced_features.md` | reference/ | **M (E + H)** | medium | Feature overviews (E) with embedded enablement snippets (H) |
| `gallery/dashboards.md` | gallery/ | **R** | high | Visual catalogue of dashboards |
| `gallery/webui.md` | gallery/ | **R** | high | Visual catalogue of WebUI screens |
| `developer/contributing.md` | developer/ | **H** | high | Contribution procedure (pragmatic; outside Diátaxis's four core quadrants) |
| `developer/CODE_OF_CONDUCT.md` | developer/ | n/a | — | Boilerplate policy |
| `developer/LICENSE.md` | developer/ | n/a | — | License text |
| `developer/reaper-batch-consolidation.md` | developer/ | **M (R + E)** | medium | Architecture before/after table (R) + design-decision rationale (E); not in nav |

---

## 2. Violations & Restructure Proposals

Each item below names the file, the violation type (per skill's anti-pattern catalogue), and a concrete split.

### V1 — `tutorial/docker_installation.md` — *Tutorial with too much reference*
**Signals:** "Available Docker images", "Ports used", "Interacting with the Docker container", "Building custom Docker images" sections are image catalogues / parameter tables, not action steps. They stall the learner's momentum by switching into lookup mode.

**Proposal — split:**
- Keep in `tutorial/`: numbered steps 1–3 (decide, pull, run), the "more future-proof" volumes variant, and the Compose example.
- Move to `reference/`:
  - "Available Docker images" subsection → `reference/docker_images.md` (or fold into `cli_env.md`).
  - "Ports used" → fold into the same reference page or `reference/ports.md`.
  - "Interacting with the Docker container" bullet list → trim and move to how-to "Common day-2 tasks" or fold into `concept/long_term_installations.md`.

### V2 — `tutorial/preparing_databases.md` — *Tutorial with too much explanation*
**Signals:** "PL/Python helpers" and "Different source types explained" sections are several paragraphs each of discursive prose. They appear mid-tutorial between two action blocks.

**Proposal — split:**
- Keep in `tutorial/`: "Basic preparations" + the `pg_stat_statements` setup checklist + the `pgwatch metric print-init | psql` pattern.
- Move to `concept/`:
  - "PL/Python helpers" prose → `concept/os_helpers.md` (explanation; cross-link from tutorial).
  - "Different source types explained" → `reference/source_types.md` (a real reference table for `kind: ...`).

### V3 — `tutorial/custom_installation.md` — *Tutorial branch dilution*
**Signals:** Step 2 "Bootstrap the configuration store" offers a Postgres-DB vs YAML-file branch with separate paragraphs; the rest of the doc does too. Diátaxis tutorials should be a single happy path; alternative paths belong in how-to.

**Proposal:** Reframe as a true tutorial:
- Pick **one** happy path (Postgres config store + Postgres sink) and walk end-to-end. Mention the YAML variant in one sentence plus a link to a new how-to "Set up pgwatch with YAML configuration files".
- Move all branching prose to `howto/yaml_configuration.md`.

### V4 — `tutorial/upgrading.md` — *Reference content inside tutorial*
**Signals:** "Updating Grafana", "Updating Grafana dashboards", "Updating the metrics collector" are short factual paragraphs that read like reference entries, not action steps. They include link-outs (good) but no concrete commands.

**Proposal:** Either tighten these into one-line "see `<doc>`" references or convert the whole file into a how-to where each sub-case has its own step sequence.

### V5 — `concept/security.md` — *Explanation with embedded how-to*
**Signals:** The whole file is conceptual except for a `docker run` snippet in "Launching a more secure Docker container". A long sample command is a how-to gesture inside explanation prose.

**Proposal:** Drop the `docker run` snippet; replace with a one-line pointer to a new `howto/harden_docker_deployment.md` that contains the full command and explains each env var.

### V6 — `concept/kubernetes.md` — *Mixed-mode borderline*
**Signals:** Three short paragraphs of context + a bare `helm install` command with no values-file explanation.

**Proposal:** Promote to `howto/deploy_to_kubernetes.md`. Keep two sentences of context, then list the steps (clone repo, inspect `values.yaml`, run `helm install`, verify). Move the "Charts not maintained by pgwatch" disclaimer to the top.

### V7 — `reference/metric_definitions.md` — *Strong mixed-mode violation*
**Signals:** Three quadrants collide in one file:
- "What is a metric?" / "What is a preset?" / "Custom metrics" prose (Explanation)
- YAML schema table and field-by-field descriptions (Reference)
- "Adding and using a custom metric" sub-section with Web UI click-path (How-to)

**Proposal — split:**
- `concept/metrics_and_presets.md` (Explanation): "What is a metric", "What is a preset", the custom-metrics design rules prose.
- `reference/metric_definitions.md` (Reference, slimmed): YAML schema table only; field-by-field spec.
- `howto/add_custom_metric.md` (How-to): Web UI + YAML workflows.

### V8 — `reference/advanced_features.md` — *Explanation + how-to + reference*
**Signals:** Each subsection (Patroni, log parsing, PgBouncer, Pgpool, Prometheus scraping, cloud presets) mixes a description with an enablement snippet (`--sink=prometheus://...`, etc.) — the snippets are how-to, the descriptions are explanation.

**Proposal — split:**
- `concept/advanced_features.md` (Explanation): What each advanced feature is and why it exists.
- One short how-to per feature that contains the actual commands: `howto/monitor_patroni_cluster.md`, `howto/enable_log_parsing.md`, `howto/monitor_pgbouncer.md`, etc.

### V9 — `reference/env_variables.md` — *Stub with overlap*
**Signals:** Only lists Docker-specific + Grafana variables. The "Gatherer daemon" section is one line that defers entirely to `cli_env.md`.

**Proposal:** Either delete and merge a short Docker-only section into `cli_env.md`, or rename to `reference/docker_variables.md` and make it the canonical home for `PW_*` Docker-specific variables that aren't gatherer CLI flags.

### V10 — `howto/dashboarding_alerting.md` — *How-to that teaches*
**Signals:** Heavy explanation ("Grafana alerting is great but clicky", "we don't want to be too opinionated"). Few concrete steps. The actionable items ("use Save as") are buried.

**Proposal:** Reframe as `concept/observability_stack.md` (Explanation) discussing Grafana's role + trade-offs; create a new `howto/set_up_alerting.md` that contains the actual steps (create data source, import "Alert Template" dashboard, add alert rule, choose channel).

### V11 — `howto/monitor_prometheus_exporter.md` — *Quick-start + reference mix*
**Signals:** "Quick Start" is how-to; "Connection String Format" + "Presets" tables are reference.

**Proposal:** Keep the YAML examples and quick-start in how-to (action-oriented: "configure pgwatch to scrape Patroni"). Move the format table and preset table to `reference/prometheus_source.md` and link.

### V12 — `howto/using_managed_services.md` — *Reference + explanation*
**Signals:** Each provider section has factual feature lists (`pg_monitor` available, Python unavailable) and contextual commentary (Azure file-access note, AWS Aurora missing metrics).

**Proposal:** Promote to `reference/managed_postgres_support.md` (Reference) — each provider becomes a row in a feature-comparison table; keep provider-specific notes as admonitions.

### V13 — `concept/web_ui.md` — *Minor: security subsection drifts toward reference*
**Signals:** "Web UI security" lists flags (`--web-user`, `PW_WEBUSER`, etc.) and is essentially a mini-reference inside an explanation doc.

**Proposal:** Drop the flag list; replace with a single sentence plus a link to `reference/cli_env.md#webui`.

---

## 3. Structural / Process Proposals

### S1 — Folder ↔ quadrant mapping is loose; fix it
The Diátaxis folders exist but contents drift. Three options:

| Option | Description | Trade-off |
|---|---|---|
| **A. Realign folders to quadrants** (recommended) | `tutorial/` = pure tutorials only · `howto/` = how-to only · `concept/` = explanation only · `reference/` = reference only · `gallery/` = visual reference · `intro/` = landing/overview (allow mixed) · `developer/` = out-of-scope policy | Strongest signal; biggest move-count |
| **B. Keep folders, add `_quadrant: …` frontmatter** | Each file declares its quadrant in YAML; a CI check enforces single quadrant | Lower-risk; weaker navigation signal |
| **C. Drop folder convention, use tags only** | mkdocs-tags plugin already loaded (see `mkdocs.yml`); tag every page by quadrant | High flexibility; but breaks any URL stability |

**Recommended: A.** Apply the splits above, then verify every folder contains only one quadrant (with `intro/` and `developer/` as the two exceptions).

### S2 — Fix the `mkdocs.yml` nav miscategorization
- `howto/sizing_recommendations.md` is filed under **Concepts** (line 56). Move it under **Reference** (recommended new home) or under **How-To Guides**. The new title in the nav should match the new file location.

### S3 — Add the orphaned doc to nav
- `developer/reaper-batch-consolidation.md` is a substantive design doc that is invisible in the published site. Either add it under **Developer** (or a new **Architecture Decision Records** section) or move it to `spec/` where other design docs already live (see `spec/design-source-failure-resilience.md`, etc.).

### S4 — Standardise YAML frontmatter
Some files have full `title:` + `…` frontmatter; others (`gallery/*.md`, `developer/CODE_OF_CONDUCT.md`, `developer/LICENSE.md`, `reference/rest.md`, `reference/sinks_options.md`, `reference/env_variables.md`, `howto/implement_grpc_server.md`) have no frontmatter. Add a minimum `title:` to every file so mkdocs renders a page title and search-indexing works correctly.

### S5 — Add a single canonical tutorial for the "day-1 happy path"
Currently `tutorial/docker_installation.md` and `tutorial/custom_installation.md` both try to be the beginner tutorial. Add a clearly-marked `tutorial/quickstart.md` that walks a brand-new user from `docker run cybertecpostgresql/pgwatch-demo` to seeing the first metrics in Grafana — a single happy path, no branching, ≤ 8 steps.

### S6 — Add CI lint for quadrant purity
A small script that:
1. Parses YAML frontmatter for an optional `quadrant:` field.
2. Runs an LLM-style classifier on each file body (out of scope here, but spec-able).
3. Fails the build if any file contains imperatives while tagged `reference` or `explanation`.

This prevents regressions after the cleanup.

### S7 — Tighten cross-linking
Many violations exist because the authors embedded explanations inline rather than linking out. After the splits, ensure every reference doc has at the top:
```
> See [Concepts → <topic>] for background.
> See [How-to → <task>] for the workflow.
> See [Tutorial → <lesson>] for the guided lesson.
```

### S8 — Establish a "developer" quadrant policy
Diátaxis doesn't define developer policy docs. Two pragmatic choices:
- Treat `developer/` as **out-of-scope** for quadrant enforcement (boilerplate policy + project contribution guides).
- Add `developer/` as a documented fifth bucket in the docs `README` (if added) and exempt it from quadrant linting.

Pick one and document it.

---

## 4. Quadrant Coverage Matrix (target state after the proposals above)

| Quadrant | Target files | Files after cleanup |
|---|---|---|
| **Explanation** | `index.md` (mixed OK), `intro/features.md`, `intro/project_background.md`, `concept/components.md`, `concept/installation_options.md`, `concept/long_term_installations.md`, `concept/web_ui.md`, `concept/metrics_and_presets.md` (split from V7), `concept/advanced_features.md` (split from V8), `concept/observability_stack.md` (split from V10) | 10 |
| **Tutorial** | `tutorial/quickstart.md` (new), `tutorial/docker_installation.md` (slimmed), `tutorial/custom_installation.md` (single path), `tutorial/preparing_databases.md` (slimmed), `tutorial/upgrading.md` (slimmed) | 5 |
| **How-to** | `howto/config_db_bootstrap.md`, `howto/metrics_db_bootstrap.md`, `howto/migrate_v2_to_v3.md`, `howto/reverse_proxy.md`, `howto/implement_grpc_server.md`, `howto/harden_docker_deployment.md` (split from V5), `howto/deploy_to_kubernetes.md` (split from V6), `howto/yaml_configuration.md` (split from V3), `howto/add_custom_metric.md` (split from V7), `howto/set_up_alerting.md` (split from V10), `howto/monitor_prometheus_exporter.md` (slimmed), `howto/monitor_patroni_cluster.md` (split from V8), `howto/enable_log_parsing.md`, `howto/monitor_pgbouncer.md` | 14 |
| **Reference** | `reference/cli_env.md`, `reference/sinks_options.md`, `reference/rest.md`, `reference/env_variables.md` (or merged into cli_env), `reference/metric_definitions.md` (slimmed), `reference/prometheus_source.md` (split from V11), `reference/managed_postgres_support.md` (promoted from V12), `reference/source_types.md` (split from V2), `reference/docker_images.md` or `reference/ports.md` (split from V1), `reference/sizing_recommendations.md` (moved from howto/) | 10 |
| **Gallery (reference, visual)** | `gallery/dashboards.md`, `gallery/webui.md` | 2 |
| **Developer (out of scope)** | `developer/contributing.md`, `developer/CODE_OF_CONDUCT.md`, `developer/LICENSE.md`, `developer/reaper-batch-consolidation.md` (added to nav) | 4 |

**Net change:** +9 files (mainly how-to granularisation); 8 existing files trimmed or split; 1 misfile moved; 1 orphaned doc added to nav.

---

## 5. Priority Order for Implementation

| Order | Item | Effort | Risk | Why |
|---|---|---|---|---|
| 1 | S2 — fix `sizing_recommendations.md` misfile in nav | trivial | none | One-line `mkdocs.yml` change |
| 2 | S3 — add `reaper-batch-consolidation.md` to nav | trivial | none | One-line `mkdocs.yml` change |
| 3 | S4 — add frontmatter to all docs | small | none | Mechanical; improves UX immediately |
| 4 | V6 — promote `concept/kubernetes.md` to `howto/deploy_to_kubernetes.md` | small | low | Cleans up a mixed-mode file in one move |
| 5 | V5 — extract hardening snippet into `howto/harden_docker_deployment.md` | small | low | High value; commonly-asked question |
| 6 | V9 — resolve `env_variables.md` stub (merge or rename) | small | low | Removes a duplicate |
| 7 | V7 — split `reference/metric_definitions.md` | medium | medium | Largest single cleanup; requires careful YAML-schema preservation |
| 8 | V1 — split `tutorial/docker_installation.md` | medium | medium | Several sub-splits; coordinate with V3 |
| 9 | V3 — single-path `tutorial/custom_installation.md` + new YAML how-to | medium | medium | Requires writing new YAML how-to |
| 10 | V8 — split `reference/advanced_features.md` into per-feature how-tos | large | medium | Largest single source-of-truth change |
| 11 | V2, V4, V10, V11, V12, V13 — remaining splits and trims | medium | low | Mostly mechanical once V1/V3/V7/V8 patterns are established |
| 12 | S5 — write `tutorial/quickstart.md` | medium | low | New content; needs review |
| 13 | S6 — quadrant CI lint | medium | low | Prevents future regressions |
| 14 | S7 — standardise cross-link preamble | small | low | Polish |
| 15 | S8 — document developer/ policy | trivial | none | One paragraph |

---

## 6. Risks & Caveats

- **Link rot:** every split changes file paths. A search-and-replace pass across all docs and `mkdocs.yml` must precede any redirect rules.
- **Search index churn:** mkdocs Material's search will re-index. Expect a brief search-quality dip immediately after the rename.
- **Translator / external link breakage:** `use_directory_urls: false` (per `mkdocs.yml`) is set, so URLs are `.html`-suffixed and stable across most moves; still verify any inbound links from blog posts or release notes.
- **Code-block continuity:** some YAML snippets span multiple sections; ensure they remain syntactically valid after relocation.
- **The skill's policy** explicitly forbids merging quadrants — every split above preserves one quadrant per file.
- **The developer docs** (`developer/contributing.md`) deliberately sit outside Diátaxis; applying quadrant rules there would over-constrain boilerplate policy.

---

## 7. Out of Scope (for this audit)

- Content accuracy review (each command, env var, flag) — a separate review pass with the project's running binary.
- Doc-string / godoc audit (`devel/godoc/index.html` is referenced from `mkdocs.yml` but lives outside `docs/`).
- Translation / i18n.
- Style-guide enforcement (sentence-case headings, etc.) — recommend a `vale` or `markdownlint` config in a follow-up.
- Image/asset review in `gallery/` and inline figures.

---

## 8. Appendix — Files Originally Outside `mkdocs.yml` Nav (since dropped)

- `docs/developer/reaper-batch-consolidation.md` — confirmed as a temporary working doc and removed from the tree (see commit removing it).

## 9. Appendix — Files with Missing YAML Frontmatter

- `docs/gallery/webui.md`
- `docs/developer/CODE_OF_CONDUCT.md`
- `docs/developer/LICENSE.md`
- `docs/developer/reaper-batch-consolidation.md`
- `docs/reference/rest.md`
- `docs/reference/sinks_options.md`
- `docs/reference/env_variables.md`
- `docs/howto/implement_grpc_server.md`
- `docs/howto/using_managed_services.md` *(verify during cleanup pass)*
- `docs/howto/sizing_recommendations.md` *(verify during cleanup pass)*

(Confirm with `head -1 <file>` for each during implementation.)
