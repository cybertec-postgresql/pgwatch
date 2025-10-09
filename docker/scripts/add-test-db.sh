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
    (name,              dbtype,         preset_config,  connstr) 
VALUES 
    ('demo',            'postgres',     'debug',        'postgresql://pgwatch:pgwatchadmin@postgres/pgwatch'),
    ('demo_metrics',    'postgres',     'full',         'postgresql://pgwatch:pgwatchadmin@postgres/pgwatch_metrics'), 
    ('demo_standby',    'postgres',     'full',         'postgresql://pgwatch:pgwatchadmin@postgres-standby/pgwatch'),
    ('demo_patroni',    'patroni',      'basic',        'etcd://etcd1:2379,etcd2:2379,etcd3:2379/service/demo'),
    ('demo_pgbouncer',  'pgbouncer',    'pgbouncer',    'postgresql://pgwatch:pgwatchadmin@pgbouncer/pgbouncer'),
    ('demo_pgpool',     'pgpool',       'pgpool',       'postgresql://pgwatch:pgwatchadmin@pgpool/pgwatch');"
