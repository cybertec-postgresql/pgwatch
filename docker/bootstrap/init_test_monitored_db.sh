#!/bin/bash

if [ -n "$PW3_TESTDB" ] ; then
  psql -v ON_ERROR_STOP=1 --username "pgwatch3" --dbname "pgwatch3" <<-EOSQL
      INSERT INTO pgwatch3.monitored_db (md_unique_name, md_preset_config_name, md_config, md_hostname, md_port, md_dbname, md_user, md_password)
      SELECT 'test', 'exhaustive', null, 'localhost', '5432', 'pgwatch3', 'pgwatch3', 'pgwatch3admin'
      WHERE NOT EXISTS (
          SELECT * FROM pgwatch3.monitored_db WHERE (md_unique_name, md_hostname, md_dbname) = ('test', 'localhost', 'pgwatch3')
      );
EOSQL
fi