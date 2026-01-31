CREATE SCHEMA IF NOT EXISTS pgwatch;

CREATE TABLE IF NOT EXISTS pgwatch.metric (
	name text PRIMARY KEY,
	sqls jsonb NOT NULL,
	init_sql text,
	description text,
	node_status text,
	gauges text[],
	is_instance_level bool NOT NULL DEFAULT FALSE,
	storage_name text
);
	
COMMENT ON COlUMN pgwatch.metric.node_status IS 'currently supports `primary` and `standby`';
COmment on column pgwatch.metric.gauges IS 'comma separated list of gauge metric columns, * if all columns are gauges';
COMMENT ON COlUMN pgwatch.metric.is_instance_level IS 'if true, the metric is collected only once per monitored instance';
COMMENT ON COlUMN pgwatch.metric.storage_name IS 'data is stored in the specified table/file/sink target instead of the default one';

CREATE TABLE IF NOT EXISTS pgwatch.preset (
	name text PRIMARY KEY,
	description text NOT NULL,
	metrics jsonb NOT NULL,
	sort_order int NOT NULL DEFAULT 0
);

CREATE OR REPLACE FUNCTION pgwatch.update_preset()
	RETURNS TRIGGER
	AS $$
BEGIN
	IF TG_OP = 'DELETE' THEN
		UPDATE pgwatch.preset 
		SET metrics = metrics - OLD.name::text 
		WHERE metrics ? OLD.name::text;
	ELSIF TG_OP = 'UPDATE' THEN
		IF OLD.name <> NEW.name THEN
			UPDATE pgwatch.preset
			SET metrics = jsonb_set(metrics - OLD.name::text, ARRAY[NEW.name::text], metrics -> OLD.name)
			WHERE metrics ? OLD.name::text;
		END IF;
	END IF;
	RETURN NULL;
END;
$$
LANGUAGE plpgsql;

CREATE TRIGGER update_preset_trigger
	AFTER DELETE OR UPDATE OF name ON pgwatch.metric
	FOR EACH ROW
	EXECUTE FUNCTION pgwatch.update_preset();


CREATE TABLE IF NOT EXISTS pgwatch.source(
	name text NOT NULL PRIMARY KEY,
	connstr text NOT NULL,
	is_superuser boolean NOT NULL DEFAULT FALSE,
	preset_config text REFERENCES pgwatch.preset(name) DEFAULT 'basic',
	config jsonb,
	is_enabled boolean NOT NULL DEFAULT 't',
	last_modified_on timestamptz NOT NULL DEFAULT now(),
	dbtype text NOT NULL DEFAULT 'postgres',
	include_pattern text, -- valid regex expected. relevant for 'postgres-continuous-discovery'
	exclude_pattern text, -- valid regex expected. relevant for 'postgres-continuous-discovery'
	custom_tags jsonb,
	"group" text NOT NULL DEFAULT 'default',
	host_config jsonb,
	only_if_master bool NOT NULL DEFAULT FALSE,
	preset_config_standby text REFERENCES pgwatch.preset (name),
	config_standby jsonb,
	CONSTRAINT preset_or_custom_config CHECK (COALESCE(preset_config, config::text) IS NOT NULL AND (preset_config IS NULL OR config IS NULL)),
	CONSTRAINT preset_or_custom_config_standby CHECK (preset_config_standby IS NULL OR config_standby IS NULL),
	CHECK (dbtype IN ('postgres', 'pgbouncer', 'postgres-continuous-discovery', 'patroni', 'pgpool')),
	CHECK ("group" ~ E'\\w+')
);

-- define migrations you need to apply
-- every change to the database schema should populate this table.
-- Version value should contain issue number zero padded followed by
-- short description of the issue\feature\bug implemented\resolved
CREATE TABLE pgwatch.migration(
    id bigint PRIMARY KEY,
    version text NOT NULL
);

INSERT INTO
    pgwatch.migration (id, version)
VALUES
    (0,  '00179 Apply metrics migrations for v3'),
    (1,  '00824 Refactor recommendations metrics to use metric_storage_name'),
    (2,  '01050 Add sort_order column to preset table');
