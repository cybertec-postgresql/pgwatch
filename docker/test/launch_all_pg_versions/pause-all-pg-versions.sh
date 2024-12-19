#!/bin/bash

for ver in {11..15} ; do
  echo "pausing PG $ver ..."
  docker pause "pg${ver}"
  docker pause "pg${ver}-repl"
done
