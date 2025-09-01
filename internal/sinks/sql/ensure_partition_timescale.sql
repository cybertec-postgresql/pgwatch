-- DROP FUNCTION IF EXISTS public.ensure_partition_timescale(text);
-- select * from public.ensure_partition_timescale('wal');

CREATE OR REPLACE FUNCTION admin.ensure_partition_timescale(
    metric text
)
RETURNS void AS
/*
  creates a top level metric table if not already existing.
  expects the "metrics_template" table to exist.
*/
$SQL$
DECLARE
    l_compression_policy text := $$
      ALTER TABLE public.%I SET (
        timescaledb.compress,
        timescaledb.compress_segmentby = 'dbname'
      );
    $$;
    l_chunk_time_interval interval;
    l_compress_chunk_interval interval;
    l_timescale_version numeric;
BEGIN
    --RAISE NOTICE 'creating partition % ...', metric;

    PERFORM pg_advisory_xact_lock(regexp_replace( md5(metric) , E'\\D', '', 'g')::varchar(10)::int8);

    IF NOT EXISTS (SELECT *
                  FROM _timescaledb_catalog.hypertable
                  WHERE table_name = metric
                    AND schema_name = 'public')
    THEN
        SELECT value::interval INTO l_chunk_time_interval FROM admin.config WHERE key = 'timescale_chunk_interval';
        IF NOT FOUND THEN
            l_chunk_time_interval := '2 days'; -- Timescale default is 7d
        END IF;

        SELECT value::interval INTO l_compress_chunk_interval FROM admin.config WHERE key = 'timescale_compress_interval';
        IF NOT FOUND THEN
            l_compress_chunk_interval := '1 day';
        END IF;

        EXECUTE format('CREATE TABLE IF NOT EXISTS public.%I (LIKE admin.metrics_template INCLUDING INDEXES)', metric);
        EXECUTE format('COMMENT ON TABLE public.%I IS $$pgwatch-generated-metric-lvl$$', metric);
        PERFORM create_hypertable(format('public.%I', metric), 'time', chunk_time_interval => l_chunk_time_interval);
        EXECUTE format(l_compression_policy, metric);
        SELECT ((regexp_matches(extversion, '\d+\.\d+'))[1])::numeric INTO l_timescale_version FROM pg_extension WHERE extname = 'timescaledb';
        IF l_timescale_version >= 2.0 THEN
          PERFORM add_compression_policy(format('public.%I', metric), l_compress_chunk_interval);
        ELSE
          PERFORM add_compress_chunks_policy(format('public.%I', metric), l_compress_chunk_interval);
        END IF;
    END IF;

END;
$SQL$ LANGUAGE plpgsql;

-- GRANT EXECUTE ON FUNCTION admin.ensure_partition_timescale(text) TO pgwatch;

CREATE OR REPLACE FUNCTION admin.ensure_partition_metric_time(
    metric text,
    metric_timestamp timestamptz,
    partitions_to_precreate int default 0,
    OUT part_available_from timestamptz,
    OUT part_available_to timestamptz)
RETURNS record AS
/*
  creates a top level metric table + time partition if not already existing.
  returns partition start/end date
*/
$SQL$
DECLARE
  l_year int;
  l_week int;
  l_doy int;
  l_part_name text;
  l_part_start date;
  l_part_end date;
  l_sql text;
  l_template_table text := 'admin.metrics_template';
BEGIN

  PERFORM pg_advisory_xact_lock(regexp_replace( md5(metric) , E'\\D', '', 'g')::varchar(10)::int8);

  IF NOT EXISTS (SELECT 1
                   FROM pg_tables
                  WHERE tablename = metric
                    AND schemaname = 'public')
  THEN
    l_sql := format('CREATE TABLE IF NOT EXISTS public.%I (LIKE admin.metrics_template INCLUDING INDEXES) PARTITION BY RANGE (time)', metric);
    EXECUTE l_sql;
    EXECUTE format('COMMENT ON TABLE public.%I IS $$pgwatch-generated-metric-lvl$$', metric);
  END IF;
  
  FOR i IN 0..partitions_to_precreate LOOP

        l_year := extract(isoyear from (metric_timestamp + '1week'::interval * i));
        l_week := extract(week from (metric_timestamp + '1week'::interval * i));

        l_part_name := format('%s_y%sw%s', metric, l_year, to_char(l_week, 'fm00' ));

        IF i = 0 THEN
            l_part_start := to_date(l_year::text || l_week::text, 'iyyyiw');
            l_part_end := l_part_start + '1week'::interval;
            part_available_from := l_part_start;
            part_available_to := l_part_end;
        ELSE
            l_part_start := l_part_start + '1week'::interval;
            l_part_end := l_part_start + '1week'::interval;
            part_available_to := l_part_end;
        END IF;

    IF NOT EXISTS (SELECT 1
                    FROM pg_tables
                    WHERE tablename = l_part_name
                      AND schemaname = 'subpartitions')
    THEN
      l_sql := format($$CREATE TABLE IF NOT EXISTS subpartitions.%I PARTITION OF public.%I FOR VALUES FROM ('%s') TO ('%s')$$,
                      l_part_name, metric, l_part_start, l_part_end);
      EXECUTE l_sql;
      EXECUTE format('COMMENT ON TABLE subpartitions.%I IS $$pgwatch-generated-metric-time-lvl$$', l_part_name);
    END IF;

  END LOOP;
END;
$SQL$ LANGUAGE plpgsql;
