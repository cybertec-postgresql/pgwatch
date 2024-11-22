#!/bin/bash

cd "$(dirname "${BASH_SOURCE[0]}")"

docker compose exec postgres psql -v ON_ERROR_STOP=1 -c \
"CREATE EXTENSION IF NOT EXISTS pg_stat_statements; 
CREATE EXTENSION IF NOT EXISTS plpython3u; 
CREATE FUNCTION get_load_average(OUT load_1min float, OUT load_5min float, OUT load_15min float) AS ' 
  from os import getloadavg 
  la = getloadavg() 
  return [la[0], la[1], la[2]]' 
LANGUAGE plpython3u VOLATILE; 
GRANT EXECUTE ON FUNCTION get_load_average() TO pgwatch; 
GRANT EXECUTE ON FUNCTION pg_stat_file(text) TO pgwatch; 
GRANT EXECUTE ON FUNCTION pg_stat_file(text, boolean) TO pgwatch; 
INSERT INTO pgwatch.source (name, preset_config, connstr) 
  SELECT 'demo', 'exhaustive', 'postgresql://pgwatch:pgwatchadmin@postgres/pgwatch' 
  WHERE NOT EXISTS (SELECT * FROM pgwatch.source WHERE name = 'demo')"
