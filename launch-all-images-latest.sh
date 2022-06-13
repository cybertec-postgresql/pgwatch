#!/bin/bash

echo "dropping old containers if any ..."
for x in pw3 pw3pg pw3nr ; do
  docker stop $x &>/dev/null
  docker rm $x &>/dev/null
done

echo "run -d --rm --name pw3pg -p 9433:5432 -p 9001:3000 -p 9081:8080 -e PW3_TESTDB=1 cybertec/pgwatch3:latest"
docker run -d --rm --name pw3pg -p 9433:5432 -p 9001:3000 -p 9081:8080 -e PW3_TESTDB=1 cybertec/pgwatch3:latest

sleep 30

PGPASSWORD=pgwatch3admin
echo "pgbench -i -s10 ..."
pgbench -i -s10 --unlogged -U pgwatch3 -p 9433 pgwatch3 &>/dev/null

echo "generating some light load for 10min ..."
pgbench -T600 -R1 -U pgwatch3 -p 9433 pgwatch3 &

wait
