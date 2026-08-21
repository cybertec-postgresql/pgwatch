---
title: Security aspects
---

## General security information

Security can be tightened for most pgwatch components quite granularly,
but the default values for the Docker image don't focus on security
though but rather on being quickly usable for ad-hoc performance
troubleshooting, which is where the roots of pgwatch lie.

Some points on security:

- The administrative Web UI doesn't have by default any security.
    Configurable via env. variables.

- Viewing Grafana dashboards by default doesn't require login.
    Editing needs a password. Configurable via env. variables.

- Dashboards based on the "stat_statements" metric (Stat Statement
    Overview / Top) expose actual queries.

    They should be "mostly" stripped of details though and replaced by
    placeholders by Postgres, but if no risks can be taken such
    dashboards (or at least according panels) should be deleted. Or as
    an alternative the `stat_statements_no_query_text` and
    `pg_stat_statements_calls` metrics could be used, which don't
    store query texts in the first place.

- Safe certificate connections to Postgres are supported. According
    *sslmode* (verify-ca, verify-full) and cert file paths
    need to be specified then in connection string on Web UI "/dbs" page
    or in the YAML config.

- Note that although pgwatch can handle password security, in many
    cases it's better to still use the standard LibPQ *.pgpass* file to
    store passwords.

For a concrete recipe that turns all of the above on at once, see
[How-to: Harden a Docker deployment](../howto/harden_docker_deployment.md).
