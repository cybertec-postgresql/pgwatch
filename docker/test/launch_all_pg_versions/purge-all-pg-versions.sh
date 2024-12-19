#!/bin/bash

for ver in {11..15} ; do
  echo "stopping PG $ver ..."
  docker stop "pg${ver}"
  docker stop "pg${ver}-repl"

  echo "removing PG $ver ..."
  docker rm "pg${ver}"
  docker rm "pg${ver}-repl"

  echo "removing volumes for PG $ver ..."
  docker volume rm "pg${ver}"
  docker volume rm "pg${ver}-repl"
done
