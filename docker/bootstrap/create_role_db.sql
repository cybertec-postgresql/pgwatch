CREATE ROLE pgwatch WITH 
    IN ROLE pg_monitor
    LOGIN PASSWORD 'pgwatchadmin';  -- change the pw for production

CREATE DATABASE pgwatch OWNER pgwatch;

CREATE DATABASE pgwatch_metrics OWNER pgwatch;

-- for remote log parsing to work in case of server_log_event_counts metric is used
GRANT EXECUTE ON FUNCTION pg_read_file(text, bigint, bigint) TO pgwatch;
