#!/bin/bash

# Enable trace mode
set -x

GIT_HASH=$(git show -s --format=%H HEAD)
GIT_TIME=$(git show -s --format=%cI HEAD)
VERSION=$(git rev-parse --abbrev-ref HEAD)
GOEXPERIMENT=greenteagc go build -ldflags "-X 'main.commit=$GIT_HASH' -X 'main.date=$GIT_TIME' -X 'main.version=$VERSION'"
