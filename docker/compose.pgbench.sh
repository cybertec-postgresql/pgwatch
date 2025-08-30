#!/bin/bash

cd "$(dirname "${BASH_SOURCE[0]}")"

docker compose exec -e PGDATABASE=pgwatch postgres sh -c \
 "pgbench --initialize --scale=50 &&
  pgbench --progress=5 --client=10 --jobs=2 --time=300 --rate=150 &&
  pgbench --initialize --init-steps=d"
