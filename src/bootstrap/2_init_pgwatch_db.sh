#!/bin/bash

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "pgwatch3" <<-EOSQL
    CREATE EXTENSION pg_qualstats;
    CREATE EXTENSION plpython3u;
EOSQL

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "pgwatch3" \
    -f /pgwatch3/metrics/00_helpers/get_load_average/9.1/metric.sql \
    -f /pgwatch3/metrics/00_helpers/get_stat_statements/9.4/metric.sql \
    -f /pgwatch3/metrics/00_helpers/get_stat_activity/9.2/metric.sql \
    -f /pgwatch3/metrics/00_helpers/get_stat_replication/9.2/metric.sql \
    -f /pgwatch3/metrics/00_helpers/get_table_bloat_approx/9.5/metric.sql \
    -f /pgwatch3/metrics/00_helpers/get_table_bloat_approx_sql/12/metric.sql \
    -f /pgwatch3/metrics/00_helpers/get_wal_size/10/metric.sql \
    -f /pgwatch3/metrics/00_helpers/get_psutil_cpu/9.1/metric.sql \
    -f /pgwatch3/metrics/00_helpers/get_psutil_mem/9.1/metric.sql \
    -f /pgwatch3/metrics/00_helpers/get_psutil_disk/9.1/metric.sql \
    -f /pgwatch3/metrics/00_helpers/get_psutil_disk_io_total/9.1/metric.sql