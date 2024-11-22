#!/bin/bash

cd "$(dirname "${BASH_SOURCE[0]}")"

export PGHOST=postgres 
export PGUSER=pgwatch 
export PGDATABASE=pgwatch 
export PGPASSWORD=pgwatchadmin

docker compose exec postgres sh -c \
 "pgbench --initialize --scale=50 &&
  pgbench --progress=5 --client=10 --jobs=2 --transactions=10000 &&
  pgbench --initialize --init-steps=d"
