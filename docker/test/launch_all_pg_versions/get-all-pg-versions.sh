#!/bin/bash

for ver in {11..15} ; do
  docker pull postgres:$ver
done
