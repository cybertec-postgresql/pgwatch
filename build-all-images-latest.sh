#!/bin/bash

echo "building build-docker-daemon (log: build-docker-daemon.log)"
./build-docker-daemon.sh &> build-docker-daemon.log
if [ $? -ne 0 ]; then
  echo "failed. see log for details"
else
  echo "ok"
fi

echo "building build-docker (log: build-docker.log)"
./build-docker.sh &> build-docker.log
if [ $? -ne 0 ]; then
  echo "failed. see log for details"
else
  echo "ok"
fi

echo "building build-docker-db-bootstrapper (log: build-docker-db-bootstrapper.log)"
./build-docker-db-bootstrapper.sh &> build-docker-db-bootstrapper.log
if [ $? -ne 0 ]; then
  echo "failed. see log for details"
else
  echo "ok"
fi

echo "done"
