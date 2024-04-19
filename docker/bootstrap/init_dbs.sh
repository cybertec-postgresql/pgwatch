#!/bin/bash

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "pgwatch3" <<-EOSQL
    CREATE EXTENSION pg_qualstats;
    CREATE EXTENSION plpython3u;
    CREATE EXTENSION pg_stat_statements;
    GRANT EXECUTE ON FUNCTION pg_stat_file(text) TO pgwatch3;
    GRANT EXECUTE ON FUNCTION pg_stat_file(text, boolean) TO pgwatch3;
EOSQL

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "pgwatch3" <<-EOSQL
BEGIN;
CREATE OR REPLACE FUNCTION get_load_average(OUT load_1min float, OUT load_5min float, OUT load_15min float) AS
'
    from os import getloadavg
    la = getloadavg()
    return [la[0], la[1], la[2]]
' LANGUAGE plpython3u VOLATILE;
GRANT EXECUTE ON FUNCTION get_load_average() TO pgwatch3;
COMMENT ON FUNCTION get_load_average() is 'created for pgwatch3';
COMMIT;
EOSQL
