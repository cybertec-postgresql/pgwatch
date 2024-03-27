#!/bin/bash

if [ -n "$PW3_TESTDB" ] ; then
  echo "adding test monitored database..."
  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "pgwatch3" <<-EOSQL
      INSERT INTO pgwatch3.source (name, preset_config, config, connstr)
      SELECT 'test', 'exhaustive', null, 'postgresql://pgwatch3:pgwatch3admin@localhost:5432/pgwatch3'
      WHERE NOT EXISTS (SELECT * FROM pgwatch3.source WHERE name = 'test');
EOSQL
fi