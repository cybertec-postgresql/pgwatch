#!/usr/bin/env bash

source ~/.bashrc

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$(dirname "$SCRIPT_DIR")"
docker build \
 --build-arg GIT_TIME=`git show -s --format=%cI HEAD` \
 --build-arg GIT_HASH=`git show -s --format=%H HEAD` \
 --build-arg VERSION=`git rev-parse --abbrev-ref HEAD` \
 -t cybertecpostgresql/pgwatch:latest \
 -f docker/Dockerfile .
