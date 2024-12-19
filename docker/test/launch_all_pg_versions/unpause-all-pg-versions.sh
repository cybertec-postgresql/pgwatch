#!/bin/bash

for ver in {11..15} ; do
  echo "unpausing PG $ver ..."
  docker unpause "pg${ver}"
  docker unpause "pg${ver}-repl"
done
