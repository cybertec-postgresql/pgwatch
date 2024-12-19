---
title: Technical details
---

Here are some technical details that might be interesting for those who
are planning to use pgwatch for critical monitoring tasks or customize
it in some way.

-   Dynamic management of monitored databases, metrics and their
    intervals - no need to restart/redeploy

    Config DB or YAML / SQL files are scanned every 2 minutes (by
    default, changeable via `--servers-refresh-loop-seconds`) and changes
    are applied dynamically. As common connectivity errors also
    handled, there should be no need to restart the gatherer "for
    fun". Please always report issues which require restarting.

-   There are some safety features built-in so that monitoring would not
    obstruct actual operation of databases

    -   Up to 2 concurrent queries per monitored database (thus more per
        cluster) are allowed
    -   Configurable statement timeouts per DB
    -   SSL connections support for safe over-the-internet monitoring
        (use `-e PW_WEBSSL=1 -e PW_GRAFANASSL=1` when launching
        Docker)
    -   Optional authentication for the Web UI and Grafana (by default
        freely accessible)

-   Instance-level metrics caching

    To further reduce load on multi-DB instances, pgwatch can cache the
    output of metrics that are marked to gather only instance-level
    data. One such metric is for example "wal", and the *metric
    attribute* is "is_instance_level". Caching will be activated only
    for *continuous* [source types](../tutorial/preparing_databases.md#different-source-types-explained), 
    and to a default limit of up to 30 seconds (changeable
    via the `--instance-level-cache-max-seconds` param).
