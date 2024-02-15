#!/bin/bash
docker build \
 --build-arg GIT_TIME=`git show -s --format=%cI HEAD` \
 --build-arg GIT_HASH=`git show -s --format=%H HEAD` \
 --build-arg VERSION=`git rev-parse --abbrev-ref HEAD` \ 
 -t cybertec/pgwatch3-demo:latest \
 -f docker/demo/Dockerfile .
