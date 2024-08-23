---
title: Security aspects
---

## General security information

Security can be tightened for most pgwatch3 components quite granularly,
but the default values for the Docker image don't focus on security
though but rather on being quickly usable for ad-hoc performance
troubleshooting, which is where the roots of pgwatch3 lie.

Some points on security:

-   The administrative Web UI doesn't have by default any security.
    Configurable via env. variables.

-   Viewing Grafana dashboards by default doesn't require login.
    Editing needs a password. Configurable via env. variables.

-   Dashboards based on the "stat_statements" metric (Stat Statement
    Overview / Top) expose actual queries.

    They should be "mostly" stripped of details though and replaced by
    placeholders by Postgres, but if no risks can be taken such
    dashboards (or at least according panels) should be deleted. Or as
    an alternative the `stat_statements_no_query_text` and
    `pg_stat_statements_calls` metrics could be used, which don't
    store query texts in the first place.

-   Safe certificate connections to Postgres are supported. According 
    *sslmode* (verify-ca, verify-full) and cert file paths
    need to be specified then in connection string on Web UI "/dbs" page 
    or in the YAML config.

-   Note that although pgwatch3 can handle password security, in many
    cases it's better to still use the standard LibPQ *.pgpass* file to
    store passwords.

## Launching a more secure Docker container

Some common sense security is built into default Docker images for all
components but not actived by default. A sample command to launch
pgwatch3 with following security "checkpoints" enabled:

1.  HTTPS for both Grafana and the Web UI with self-signed certificates
1.  No anonymous viewing of graphs in Grafana
1.  Custom user / password for the Grafana "admin" account
1.  No anonymous access / editing over the admin Web UI
1.  No viewing of internal logs of components running inside Docker
1.  Password encryption for connect strings stored in the Config DB


    ```properties
    docker run --name pw3 -d --restart=unless-stopped \
      -p 3000:3000 -p 8080:8080 \
      -e PW_GRAFANASSL=1 -e PW_WEBSSL=1 \
      -e PW_GRAFANANOANONYMOUS=1 -e PW_GRAFANAUSER=myuser \
      -e PW_GRAFANAPASSWORD=mypass \
      -e PW_WEBNOANONYMOUS=1 -e PW_WEBNOCOMPONENTLOGS=1 \
      -e PW_WEBUSER=myuser -e PW_WEBPASSWORD=mypass \
      -e PW_AES_GCM_KEYPHRASE=qwerty \
      cybertec/pgwatch3
    ```

For custom installs it's up to the user though. A hint - Docker
*launcher* files can also be inspected to see which config parameters
are being touched.
