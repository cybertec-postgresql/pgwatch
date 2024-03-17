CREATE SCHEMA IF NOT EXISTS pgwatch3 AUTHORIZATION pgwatch3;

SET search_path TO pgwatch3, public;

-- role/db create script is in bootstrap/create_db_pgwatch.sql
SET ROLE TO pgwatch3;

-- drop table if exists preset_config cascade;
/* preset configs for typical usecases */
CREATE TABLE IF NOT EXISTS pgwatch3.preset_config (
    pc_name text PRIMARY KEY,
    pc_description text NOT NULL,
    pc_config jsonb NOT NULL,
    pc_last_modified_on timestamptz NOT NULL DEFAULT now()
);

-- drop table if exists pgwatch3.monitored_db;
CREATE TABLE IF NOT EXISTS pgwatch3.monitored_db (
    md_name text NOT NULL PRIMARY KEY,
    md_connstr text NOT NULL,
    md_is_superuser boolean NOT NULL DEFAULT FALSE,
    md_preset_config_name text REFERENCES pgwatch3.preset_config (pc_name) DEFAULT 'basic',
    md_config jsonb,
    md_is_enabled boolean NOT NULL DEFAULT 't',
    md_last_modified_on timestamptz NOT NULL DEFAULT now(),
    md_dbtype text NOT NULL DEFAULT 'postgres',
    md_include_pattern text, -- valid regex expected. relevant for 'postgres-continuous-discovery'
    md_exclude_pattern text, -- valid regex expected. relevant for 'postgres-continuous-discovery'
    md_custom_tags jsonb,
    md_group text NOT NULL DEFAULT 'default',
    md_encryption text NOT NULL DEFAULT 'plain-text',
    md_host_config jsonb,
    md_only_if_master bool NOT NULL DEFAULT FALSE,
    md_preset_config_name_standby text REFERENCES pgwatch3.preset_config (pc_name),
    md_config_standby jsonb,
    CONSTRAINT no_colon_on_unique_name CHECK (md_name !~ ':'),
    CHECK (md_dbtype IN ('postgres', 'pgbouncer', 'postgres-continuous-discovery', 'patroni', 'patroni-continuous-discovery', 'patroni-namespace-discovery', 'pgpool')),
    CHECK (md_group ~ E'\\w+'),
    CHECK (md_encryption IN ('plain-text', 'aes-gcm-256'))
);

ALTER TABLE pgwatch3.monitored_db
    ADD CONSTRAINT preset_or_custom_config CHECK ((NOT (md_preset_config_name IS NULL AND md_config IS NULL)) AND NOT (md_preset_config_name IS NOT NULL AND md_config IS NOT NULL)),
    ADD CONSTRAINT preset_or_custom_config_standby CHECK (NOT (md_preset_config_name_standby IS NOT NULL AND md_config_standby IS NOT NULL));

CREATE TABLE IF NOT EXISTS pgwatch3.metric (
    m_id serial PRIMARY KEY,
    m_name text NOT NULL,
    m_pg_version_from int NOT NULL,
    m_sql text NOT NULL,
    m_comment text,
    m_is_active boolean NOT NULL DEFAULT 't',
    m_is_helper boolean NOT NULL DEFAULT 'f',
    m_last_modified_on timestamptz NOT NULL DEFAULT now(),
    m_master_only bool NOT NULL DEFAULT FALSE,
    m_standby_only bool NOT NULL DEFAULT FALSE,
    m_column_attrs jsonb, -- currently only useful for Prometheus
    m_sql_su text DEFAULT '',
    UNIQUE (m_name, m_pg_version_from, m_standby_only),
    CHECK (NOT (m_master_only AND m_standby_only)),
    CHECK (m_name ~ E'^[a-z0-9_\\.]+$')
);

CREATE OR REPLACE FUNCTION pgwatch3.update_preset_config ()
    RETURNS TRIGGER
    AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        UPDATE
            pgwatch3.preset_config
        SET
            pc_config = pc_config - OLD.m_name::text,
            pc_last_modified_on = now()
        WHERE
            pc_config ? OLD.m_name::text;
    ELSIF TG_OP = 'UPDATE' THEN
        IF OLD.m_name <> NEW.m_name THEN
            UPDATE
                pgwatch3.preset_config
            SET
                pc_config = jsonb_set(pc_config - OLD.m_name::text, ARRAY[NEW.m_name::text], pc_config -> OLD.m_name),
                pc_last_modified_on = now()
            WHERE
                pc_config ? OLD.m_name::text;
        END IF;
    END IF;
    RETURN NULL;
END;
$$
LANGUAGE plpgsql;

CREATE TRIGGER update_preset_config_trigger
    AFTER DELETE OR UPDATE OF m_name ON pgwatch3.metric
    FOR EACH ROW
    EXECUTE FUNCTION pgwatch3.update_preset_config ();

CREATE TABLE IF NOT EXISTS metric_attribute (
    ma_metric_name text NOT NULL PRIMARY KEY,
    ma_last_modified_on timestamptz NOT NULL DEFAULT now(),
    ma_metric_attrs jsonb NOT NULL,
    CHECK (ma_metric_name ~ E'^[a-z0-9_\\.]+$')
);

