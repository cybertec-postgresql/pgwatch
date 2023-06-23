#!/bin/bash
docker build \
 --build-arg GIT_TIME=`git show -s --format=%cI HEAD` \
 --build-arg GIT_HASH=`git show -s --format=%H HEAD` \
 -t cybertec/pgwatch3-daemon:latest \
 -f docker/daemon/Dockerfile .
