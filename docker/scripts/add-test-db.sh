#!/bin/bash

cd "$(dirname "${BASH_SOURCE[0]}")/.."

export MSYS_NO_PATHCONV=1

# We want to pipe the output of the `pgwatch metric print-init` command to the `psql` command
docker compose exec -T pgwatch /pgwatch/pgwatch metric print-init debug | \
docker compose exec -T -i postgres psql -d pgwatch -v ON_ERROR_STOP=1

docker compose exec -T pgwatch /pgwatch/pgwatch metric print-init full | \
docker compose exec -T -i postgres psql -d pgwatch_metrics -v ON_ERROR_STOP=1

docker compose exec -T postgres psql -d pgwatch -v ON_ERROR_STOP=1 -c \
"TRUNCATE pgwatch.source CASCADE;
INSERT INTO pgwatch.source 
    (name,                  dbtype,         preset_config,  connstr,                                                                    custom_tags) 
VALUES 
    ('demo',                'postgres',     'debug',        'postgresql://pgwatch:pgwatchadmin@postgres/pgwatch',                        NULL),
    ('demo_metrics',        'postgres',     'full',         'postgresql://pgwatch:pgwatchadmin@postgres/pgwatch_metrics',                NULL), 
    ('demo_standby',        'postgres',     'full',         'postgresql://pgwatch:pgwatchadmin@postgres-standby/pgwatch',                NULL),
    ('demo_patroni',        'patroni',      'basic',        'etcd://etcd1:2379,etcd2:2379,etcd3:2379/service/demo',                     NULL),
    ('demo_pgbouncer',      'pgbouncer',    'pgbouncer',    'postgresql://pgwatch:pgwatchadmin@pgbouncer/pgbouncer',                     NULL),
    ('demo_pgpool',         'pgpool',       'pgpool',       'postgresql://pgwatch:pgwatchadmin@pgpool/pgwatch',                         NULL),
    ('patroni1-prom',       'prometheus',   'patroni',      'http://patroni1:8008/metrics',                                             '{\"cluster\": \"demo\", \"node\": \"patroni1\"}'),
    ('patroni2-prom',       'prometheus',   'patroni',      'http://patroni2:8008/metrics',                                             '{\"cluster\": \"demo\", \"node\": \"patroni2\"}'),
    ('patroni3-prom',       'prometheus',   'patroni',      'http://patroni3:8008/metrics',                                             '{\"cluster\": \"demo\", \"node\": \"patroni3\"}');"
