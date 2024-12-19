#!/bin/bash

for ver in {11..15} ; do
  echo "stopping PG $ver ..."
  docker stop "pg${ver}"
  docker stop "pg${ver}-repl"
done
