---
title: Expose metrics for Prometheus scraping
---

This how-to puts pgwatch behind a Prometheus scrape endpoint instead of a database sink. It is the right choice when an external Prometheus server is already the system of record for your monitoring data.

For background, see [Advanced features → Prometheus scraping](../concept/advanced_features.md#prometheus-scraping).

## Prerequisites

- An external Prometheus server with network reachability to the pgwatch host
- At least one source configured and enabled (so there are metrics to expose)

## Steps

1. Configure your sources and metrics as usual via `--sources` / `--metrics` or the Config DB. Every metric with a positive fetch interval will be exposed; metrics with interval `0` are skipped.

2. Add a Prometheus sink to the gatherer:

    ```bash
    pgwatch \
      --sources=postgresql://pgwatch@localhost/pgwatch \
      --sink=prometheus://:9090/pgwatch
    ```

    Format: `--sink=prometheus://<host>:<port>/<namespace>`. If `<host>` is omitted the server listens on every interface; if `<namespace>` is omitted, it defaults to `pgwatch`.

3. From the Prometheus server, add a scrape job:

    ```yaml
    scrape_configs:
      - job_name: pgwatch
        static_configs:
          - targets: ['pgwatch-host:9090']
    ```

4. Verify Prometheus is pulling samples:

    ```bash
    curl http://pgwatch-host:9090/metrics | head -20
    ```

    You should see Prometheus exposition lines for each enabled metric, tagged with the source name and any custom tags.

## What is and isn't exposed

- **Exposed**: every metric with a fetch interval greater than zero. Numeric columns become Prometheus samples; `text` columns become labels; `tag_*` columns are always preserved as labels.
- **Dropped**: any metric that requires state to be stored between scrapes (notably `change_events`) is skipped.
- **Aggregated per source**: all metrics from all databases of a source share the source's labels. To distinguish individual databases, add `custom_tags`.

## See also

- [Reference: Sinks — Prometheus](../reference/sinks_options.md#prometheus) — URI format details
