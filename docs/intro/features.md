---
title: List of main features
---

-   Non-invasive setup on PostgreSQL side - no extensions nor superuser
    rights are required for the base functionality so that even
    unprivileged users like developers can get a good overview of
    database activities without any hassle
-   Lots of preset metric configurations covering all performance
    critical PostgreSQL internal Statistics Collector data
-   Intuitive metrics presentation using a set of predefined dashboards
    for the very popular Grafana dashboarding engine with optional
    alerting support
-   Easy extensibility of metrics which are defined in pure SQL, thus
    they could also be from the business domain
-   Many metric data storage options - PostgreSQL, PostgreSQL with the
    compression enabled TimescaleDB extension, Prometheus scraping, or 
    gRPC-based custom storage integration
-   Multiple deployment options - PostgreSQL configuration DB, YAML or
    ENV configuration
-   Possible to monitoring all, single or a subset (list or regex) of
    databases of a PostgreSQL instance
-   Global or per DB configuration of metrics and metric fetching
    intervals
-   Kubernetes/OpenShift ready with sample templates and a Helm chart
-   PgBouncer, Pgpool2, AWS RDS and Patroni support with automatic
    member discovery
-   Internal REST API to monitor metrics gathering status remotely
-   Built-in security with SSL connections support for all components
    and passwords encryption for connect strings
-   Very low resource requirements for the collector even when
    monitoring hundreds of instances
-   Capabilities to go beyond PostgreSQL metrics gathering with built-in
    log parsing for error detection and OS level metrics collection via
    PL/Python "helper" stored procedures