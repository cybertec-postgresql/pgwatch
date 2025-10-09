#!/bin/bash

cd "$(dirname "${BASH_SOURCE[0]}")/.."

docker compose exec postgres pg_activity -d pgwatch
