/*
  ensure_partition_timescale creates a top level metric table if not already existing
*/
CREATE OR REPLACE FUNCTION admin.ensure_partition_timescale(metric text)
RETURNS void AS
$SQL$
DECLARE
    l_chunk_time_interval interval;
    l_compress_chunk_interval interval;
BEGIN
  IF to_regclass('public.' || quote_ident(metric)) IS NOT NULL THEN
    RETURN; -- already exists
  END IF;
  PERFORM pg_advisory_xact_lock(regexp_replace( md5(metric) , E'\\D', '', 'g')::varchar(10)::int8);
  SELECT COALESCE(value::interval, '2 days') INTO l_chunk_time_interval FROM admin.config WHERE key = 'timescale_chunk_interval';
  SELECT COALESCE(value::interval, '1 day') INTO l_compress_chunk_interval FROM admin.config WHERE key = 'timescale_compress_interval';
  EXECUTE format('CREATE TABLE IF NOT EXISTS public.%I (LIKE admin.metrics_template INCLUDING INDEXES)', metric);
  EXECUTE format('COMMENT ON TABLE public.%I IS $$pgwatch-generated-metric-lvl$$', metric);
  PERFORM create_hypertable(format('public.%I', metric), 'time', chunk_time_interval => l_chunk_time_interval);
  EXECUTE format('ALTER TABLE public.%I SET (timescaledb.compress, timescaledb.compress_segmentby = $$dbname$$)', metric);
  PERFORM add_compression_policy(format('public.%I', metric), l_compress_chunk_interval);
END;
$SQL$ LANGUAGE plpgsql;
