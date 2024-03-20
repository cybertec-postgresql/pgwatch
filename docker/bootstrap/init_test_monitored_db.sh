#!/bin/bash

if [ -n "$PW3_TESTDB" ] ; then
  psql -v ON_ERROR_STOP=1 --username "pgwatch3" --dbname "pgwatch3" <<-EOSQL
      INSERT INTO pgwatch3.source (name, preset_config_name, config, connstr)
      SELECT 'test', 'exhaustive', null, 'postgresql://pgwatch3:pgwatch3admin@localhost:5432/pgwatch3'
      WHERE NOT EXISTS (SELECT * FROM pgwatch3.monitored_db WHERE name = 'test');
EOSQL
fi