#!/bin/bash
docker build --no-cache -t cybertec/pgwatch3-db-bootstrapper:latest -f docker/Dockerfile-db-bootstrapper .
