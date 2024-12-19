CREATE ROLE pgwatch WITH 
    IN ROLE pg_monitor
    LOGIN PASSWORD 'pgwatchadmin';  -- change the pw for production

CREATE DATABASE pgwatch OWNER pgwatch;

CREATE DATABASE pgwatch_grafana OWNER pgwatch;

CREATE DATABASE pgwatch_metrics OWNER pgwatch;