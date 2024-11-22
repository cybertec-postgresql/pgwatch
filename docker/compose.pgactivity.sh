#!/bin/bash

cd "$(dirname "${BASH_SOURCE[0]}")"

export PGDATABASE=pgwatch 

docker compose exec postgres pg_activity
