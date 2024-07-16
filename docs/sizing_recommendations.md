---
title: Sizing recommendations
---

-   Min 1GB of RAM is required for a Docker setup using Postgres to
    store metrics.

    The gatherer alone needs typically less than 50 MB if the metric 
    measurements are stored online. Memory consumption will increase a lot when the
    metrics store is offline though, as then metrics are cached in RAM
    in ringbuffer style up to a limit of 10k data points (for all
    databases) and then memory consumption is dependent on how "wide"
    are the metrics gathered.

-   Storage requirements vary a lot and are hard to predict.

    10GB of disk space should be enough though for monitoring a single
    DB with "exhaustive" *preset* for 1 month with Postgres storage. 2
    weeks is also the default metrics retention policy for Postgres
    running in Docker (configurable). Depending on the amount of schema
    objects - tables, indexes, stored procedures and especially on
    number of unique SQL-s, it could be also much more. If disk size
    reduction is wanted for PostgreSQL storage then best would be to use
    the TimescaleDB extension - it has built-in compression and disk
    footprint is x5 time less than vanila Postgres, while retaining full
    SQL support.

-   A low-spec (1 vCPU, 2 GB RAM) cloud machine can easily monitor 100
    DBs in "exhaustive" settings (i.e. almost all metrics are
    monitored in 1-2min intervals) without breaking a sweat (\<20%
    load).

-   A single Postgres node should handle thousands of requests per
    second.

-   When high metrics write latency is problematic (e.g. using a DBaaS
    across the Atlantic) then increasing the default maximum batching
    delay of 250ms usually gives good results.
    Relevant params: `--batching-delay-ms / PW3_BATCHING_MAX_DELAY_MS`.

-   Note that when monitoring a very large number of databases, it's
    possible to "shard" / distribute them between many metric
    collection instances running on different hosts, via the `group`
    attribute. This requires that some hosts have been assigned a
    non-default *group* identifier, which is just a text field exactly
    for this sharding purpose.
    Relevant params: `--group / PW3_GROUP`.
