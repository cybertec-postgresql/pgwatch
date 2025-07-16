#!/usr/bin/env bash

source ~/.bashrc

cd "$(dirname "${BASH_SOURCE[0]}")"

docker compose exec postgres pg_activity -d pgwatch
