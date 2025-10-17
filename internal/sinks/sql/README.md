## Rollout sequence

pgwatch automatically handles database schema for metric measurements. If `timescale` extension is available then `timescale`
schema will be used by default. Otherwise `metric-dbname-time` schema is applied.

If one wants to init the schema without running the monitoring, they should use `--init` command-line parameter, e.g.

```sh
pgwatch --config=postgresql://pgwatch:pgwatchadmin@localhost/pgwatch --sink=postgresql://pgwatch:pgwatchadmin@localhost:5432/pgwatch_metrics --init
```

## Schema types

### metric-dbname-time

A single top level table for each distinct metric in the "public" schema + 2 levels of subpartitions ("dbname" + configurable time based) in the "subpartitions" schema.

Provides the fastest query runtimes when having long retention intervals / lots of metrics data or slow disks and accessing mostly only a single DB's metrics at a time.

**Partition Interval Configuration:**
- Default: `1 week` (weekly partitions)
- **Initial Configuration**: Set via CLI/Environment variables during database initialization
  - CLI: `--partition-interval="1 day"` or `--partition-interval="3 days"`
  - Environment: `PW_PARTITION_INTERVAL="1 day"` or `PW_PARTITION_INTERVAL="3 days"`
  - **Note**: Only applied when creating a new database or when `admin` schema doesn't exist
- **Runtime Configuration**: Change via `admin.change_postgres_partition_interval(interval)` function
- **Supported intervals**: 
  - **Standard**: `1 day`, `1 week`, `1 month`, or `1 year`
  - **Custom**: Any PostgreSQL interval between 1 hour and 1 year (e.g., `2 hours`, `6 hours`, `12 hours`, `2 days`, `3 days`)
  - **Prohibited**: Minute and second-based intervals are not allowed

**Initial Configuration Examples:**
```bash
# Standard intervals via CLI
pgwatch --partition-interval="1 day" --sink=postgresql://user:pass@localhost/metrics

# Custom intervals via CLI
pgwatch --partition-interval="3 days" --sink=postgresql://user:pass@localhost/metrics

# Environment variables
export PW_PARTITION_INTERVAL="1 day"
# OR
export PW_PARTITION_INTERVAL="3 days"
pgwatch --sink=postgresql://user:pass@localhost/metrics

# Docker
docker run -e PW_PARTITION_INTERVAL="3 days" pgwatch

# Docker Compose
services:
  pgwatch:
    environment:
      PW_PARTITION_INTERVAL: "3 days"
    command:
      - "--partition-interval=3 days"
```

**Runtime Configuration Examples:**
```sql
-- Standard intervals
SELECT admin.change_postgres_partition_interval('1 day');
SELECT admin.change_postgres_partition_interval('1 week');
SELECT admin.change_postgres_partition_interval('1 month');
SELECT admin.change_postgres_partition_interval('1 year');

-- Custom intervals
SELECT admin.change_postgres_partition_interval('3 days');
SELECT admin.change_postgres_partition_interval('6 hours');
SELECT admin.change_postgres_partition_interval('12 hours');
```

Also note that when having extremely many hosts under monitoring it might be necessary to increase the `max_locks_per_transaction`
parameter in `postgresql.conf` on the metric measurements database for automatic old partition dropping to work. One could of course also drop old
data partitions with some custom script / Cron when increasing `max_locks_per_transaction` is not wanted, and actually this
kind of approach is also working behind the scenes for versions above v1.8.1.

Something like below will be done by the gatherer AUTOMATICALLY:

```sql
create table public."mymetric"
  (LIKE admin.metrics_template)
  PARTITION BY LIST (dbname);
COMMENT ON TABLE public."mymetric" IS 'pgwatch-generated-metric-lvl';

create table subpartitions."mymetric_mydbname"
  PARTITION OF public."mymetric"
  FOR VALUES IN ('my-dbname') PARTITION BY RANGE (time);
COMMENT ON TABLE subpartitions."mymetric_mydbname" IS 'pgwatch-generated-metric-dbname-lvl';

create table subpartitions."mymetric_mydbname_y2019w01" -- time calculated dynamically based on configured interval
  PARTITION OF subpartitions."mymetric_mydbname"
  FOR VALUES FROM ('2019-01-01') TO ('2019-01-07');
COMMENT ON TABLE subpartitions."mymetric_mydbname_y2019w01" IS 'pgwatch-generated-metric-dbname-time-lvl';

-- For daily partitions (if configured):
-- create table subpartitions."mymetric_mydbname_20190101"
--   PARTITION OF subpartitions."mymetric_mydbname"
--   FOR VALUES FROM ('2019-01-01') TO ('2019-01-02');

-- For monthly partitions (if configured):
-- create table subpartitions."mymetric_mydbname_201901"
--   PARTITION OF subpartitions."mymetric_mydbname"
--   FOR VALUES FROM ('2019-01-01') TO ('2019-02-01');

```

### timescale

Most suitable storage schema when using long retention periods or hundreds of databases due to built-in extra compression.
Typical compression ratios vary from 3 to 10x and also querying of larger historical data sets is typically faster.

Assumes TimescaleDB (v1.7+) extension and "outsources" partition management for normal metrics to the extensions.
Additionally one can also tune the chunking and historic data compression intervals - by default it's 2 days and 1 day. To change use the
`admin.timescale_change_chunk_interval()` and `admin.timescale_change_compress_interval()` functions.

Note that if wanting to store a deeper history of 6 months or a year then additionally using [Continuous Aggregates](https://docs.timescale.com/latest/using-timescaledb/continuous-aggregates)
might be a good idea. This will though also require modifying the Grafana dashboards, so it's out of scope for pgwatch.

Something like below will be done by the gatherer AUTOMATICALLY via the `admin.ensure_partition_timescale()` function:

```sql
CREATE TABLE public."some_metric"
  (LIKE admin.metrics_template INCLUDING INDEXES);
COMMENT ON TABLE public."some_metric" IS 'pgwatch-generated-metric-lvl';

ALTER TABLE some_metric SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'dbname'
);

SELECT add_compression_policy('some_metric', INTERVAL '1 day');
```

## Data size considerations

When you're planning to monitor lots of databases or with very low intervals, i.e. generating a lot of data, but not selecting
all of it actively (alerting / Grafana) then it would make sense to consider BRIN indexes to save a lot on storage space. See
the according commented out line in the table template definition file.

## Notice on Grafana access to metric data and GRANT-s

For more security sensitive environments where a lot of people have access to metrics you'd want to secure things a bit by
creating a separate read-only user for queries generated by Grafana. And to make sure that this user, here "pgwatch_grafana",
has access to all current and future tables in the metric DB you'd probably want to execute something like that:

```sql
ALTER DEFAULT PRIVILEGES FOR ROLE pgwatch IN SCHEMA public GRANT SELECT ON TABLES TO pgwatch_grafana;
ALTER DEFAULT PRIVILEGES FOR ROLE pgwatch IN SCHEMA subpartitions GRANT SELECT ON TABLES TO pgwatch_grafana;

GRANT USAGE ON SCHEMA public TO pgwatch_grafana;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO pgwatch_grafana;

GRANT USAGE ON SCHEMA admin TO pgwatch_grafana;
GRANT SELECT ON ALL TABLES IN SCHEMA admin TO pgwatch_grafana;

GRANT USAGE ON SCHEMA subpartitions TO pgwatch_grafana;
GRANT SELECT ON ALL TABLES IN SCHEMA subpartitions TO pgwatch_grafana;
```
