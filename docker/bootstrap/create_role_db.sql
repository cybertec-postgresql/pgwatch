CREATE ROLE pgwatch3 WITH 
    IN ROLE pg_monitor
    LOGIN PASSWORD 'pgwatch3admin';  -- change the pw for production

CREATE DATABASE pgwatch3 OWNER pgwatch3;

CREATE DATABASE pgwatch3_grafana OWNER pgwatch3;

CREATE DATABASE pgwatch3_metrics OWNER pgwatch3;