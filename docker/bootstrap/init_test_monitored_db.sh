#!/bin/bash

if [ -n "$PW_TESTDB" ] ; then
  echo "adding test monitored database..."
  psql -v ON_ERROR_STOP=1 --username "postgres" --dbname "pgwatch" <<-EOSQL
      INSERT INTO pgwatch.source (name, preset_config, config, connstr)
      SELECT 'test', 'exhaustive', null, 'postgresql://pgwatch:pgwatchadmin@localhost:5432/pgwatch'
      WHERE NOT EXISTS (SELECT * FROM pgwatch.source WHERE name = 'test');
EOSQL
fi