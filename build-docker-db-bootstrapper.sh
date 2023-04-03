#!/bin/bash
docker build \
 -t cybertec/pgwatch3-db-bootstrapper:latest \
 -f docker/Dockerfile-db-bootstrapper .
