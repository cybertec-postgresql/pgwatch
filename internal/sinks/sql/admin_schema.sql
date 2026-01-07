/*
 "admin" schema - stores schema type, partition templates and data cleanup functions
 "public" schema - top level metric tables
 "subpartitions" schema - subpartitions of "public" schema top level metric tables (if using time / dbname-time partitioning)
*/

CREATE SCHEMA IF NOT EXISTS "admin";
CREATE SCHEMA IF NOT EXISTS "subpartitions";

CREATE EXTENSION IF NOT EXISTS btree_gin;

CREATE OR REPLACE FUNCTION admin.get_default_storage_type() RETURNS text as
$$
 SELECT CASE 
  WHEN EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
    'timescale' 
  ELSE 
    'postgres' 
  END;
$$
LANGUAGE SQL;

CREATE TABLE admin.storage_schema_type (
  schema_type     text        NOT NULL DEFAULT admin.get_default_storage_type(),
  initialized_on  timestamptz NOT NULL DEFAULT now(),
  check (schema_type in ('postgres', 'timescale'))
);

INSERT INTO admin.storage_schema_type DEFAULT values;

COMMENT ON TABLE admin.storage_schema_type IS 'identifies storage schema for other pgwatch components';

CREATE UNIQUE INDEX max_one_row ON admin.storage_schema_type ((1));

/* for the Grafana drop-down. managed by the gatherer */
CREATE TABLE admin.all_distinct_dbname_metrics (
  dbname      text        NOT NULL,
  metric      text        NOT NULL,
  created_on  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (dbname, metric)
);

/* currently only used to store TimescaleDB chunk interval */
CREATE TABLE admin.config (
    key               text        NOT NULL PRIMARY KEY,
    value             text        NOT NULL,
    created_on        timestamptz NOT NULL DEFAULT now(),
    last_modified_on  timestamptz
);

-- to later change the value call the admin.change_timescale_chunk_interval(interval) function!
-- as changing the row directly will only be effective for completely new tables (metrics).
INSERT INTO admin.config (key, value) VALUES 
  ('timescale_chunk_interval', '2 days'),
  ('timescale_compress_interval', '1 day');

CREATE OR REPLACE FUNCTION trg_config_modified() RETURNS trigger
AS $$
BEGIN
  new.last_modified_on = now();
  RETURN new;
END;
$$
LANGUAGE plpgsql;

CREATE TRIGGER config_modified BEFORE UPDATE ON admin.config
FOR EACH ROW EXECUTE FUNCTION trg_config_modified();

CREATE TABLE admin.metrics_template (
  time      timestamptz NOT NULL DEFAULT now(),
  dbname    text        NOT NULL,
  data      jsonb       NOT NULL,
  tag_data  jsonb,
  CHECK (false)
);

COMMENT ON TABLE admin.metrics_template IS 'used as a template for all new metric definitions';

CREATE INDEX ON admin.metrics_template (dbname, time);
-- create index on admin.metrics_template using brin (dbname, time);  /* consider BRIN instead for large data amounts */
-- create index on admin.metrics_template using gin (tag_data) where tag_data notnull;

/*
  Define migrations you need to apply. Every change to the 
  database schema should populate this table.
  Version value should contain issue number zero padded followed by
  short description of the issue\feature\bug implemented\resolved
*/
CREATE TABLE admin.migration(
    id bigint PRIMARY KEY,
    version text NOT NULL
);

INSERT INTO
    admin.migration (id, version)
VALUES
    (0,  '01110 Apply postgres sink schema migrations');
