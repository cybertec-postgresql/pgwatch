#!/usr/bin/env bash

source ~/.bashrc

cd "$(dirname "${BASH_SOURCE[0]}")"

export MSYS_NO_PATHCONV=1

# We want to pipe the output of the `pgwatch metric print-init` command to the `psql` command
docker compose exec -T pgwatch /pgwatch/pgwatch metric print-init full | \
docker compose exec -T -i postgres psql -d pgwatch -v ON_ERROR_STOP=1

docker compose exec -T postgres psql -d pgwatch -v ON_ERROR_STOP=1 -c \
"INSERT INTO pgwatch.source (name, preset_config, connstr) VALUES 
    ('demo', 'full', 'postgresql://pgwatch:pgwatchadmin@postgres/pgwatch'), 
    ('demo_standby', 'full', 'postgresql://pgwatch:pgwatchadmin@postgres-standby/pgwatch')"



