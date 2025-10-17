-- DROP FUNCTION admin.ensure_partition_metric_dbname_time(text,text,timestamp with time zone,integer);
-- select * from admin.ensure_partition_metric_dbname_time('wal', 'kala', now());

CREATE OR REPLACE FUNCTION admin.ensure_partition_metric_dbname_time(
    metric text,
    dbname text,
    metric_timestamp timestamptz,
    partitions_to_precreate int default 3,
    OUT part_available_from timestamptz,
    OUT part_available_to timestamptz)
RETURNS record AS
/*
  creates a top level metric table, a dbname partition and a time partition if not already existing.
  returns time partition start/end date
*/
$SQL$
DECLARE
  l_year int;
  l_week int;
  l_doy  int;
  l_part_name_2nd text;
  l_part_name_3rd text;
  l_part_start date;
  l_part_end date;
  l_sql text;
  ideal_length int;
  l_template_table text := 'admin.metrics_template';
  l_partition_interval interval;
  MAX_IDENT_LEN CONSTANT integer := current_setting('max_identifier_length')::int;
BEGIN

  PERFORM pg_advisory_xact_lock(regexp_replace( md5(metric) , E'\\D', '', 'g')::varchar(10)::int8);

  -- Get the configurable partition interval
  SELECT value::interval INTO l_partition_interval FROM admin.config WHERE key = 'postgres_partition_interval';
  IF NOT FOUND THEN
    l_partition_interval := '1 week'; -- Default fallback
  END IF;

  -- 1. level
  IF NOT EXISTS (SELECT 1
                   FROM pg_tables
                  WHERE tablename = metric
                    AND schemaname = 'public')
  THEN
    -- RAISE NOTICE 'creating partition % ...', metric;
    EXECUTE format('CREATE TABLE IF NOT EXISTS public.%I (LIKE admin.metrics_template INCLUDING INDEXES) PARTITION BY LIST (dbname)', metric);
    EXECUTE format('COMMENT ON TABLE public.%I IS $$pgwatch-generated-metric-lvl$$', metric);
  END IF;

  -- 2. level

  l_part_name_2nd := metric || '_' || dbname;

  IF char_length(l_part_name_2nd) > MAX_IDENT_LEN     -- use "dbname" hash instead of name for overly long ones
  THEN
    ideal_length = MAX_IDENT_LEN - char_length(format('%s_', metric));
    l_part_name_2nd := metric || '_' || substring(md5(dbname) from 1 for ideal_length);
  END IF;

  IF NOT EXISTS (SELECT 1
                   FROM pg_tables
                  WHERE tablename = l_part_name_2nd
                    AND schemaname = 'subpartitions')
  THEN
    --RAISE NOTICE 'creating partition % ...', l_part_name_2nd; 
    EXECUTE format('CREATE TABLE IF NOT EXISTS subpartitions.%I PARTITION OF public.%I FOR VALUES IN (%L) PARTITION BY RANGE (time)',
                    l_part_name_2nd, metric, dbname);
    EXECUTE format('COMMENT ON TABLE subpartitions.%s IS $$pgwatch-generated-metric-dbname-lvl$$', l_part_name_2nd);
  END IF;

  -- 3. level
  FOR i IN 0..partitions_to_precreate LOOP

      IF i = 0 THEN
          l_part_start := date_trunc('day', metric_timestamp);
          l_part_end := l_part_start + l_partition_interval;
          part_available_from := l_part_start;
          part_available_to := l_part_end;
      ELSE
          l_part_start := l_part_start + l_partition_interval;
          l_part_end := l_part_start + l_partition_interval;
          part_available_to := l_part_end;
      END IF;

      -- Generate partition name based on interval type
      -- Check if interval is single day (1 day only)
      IF l_partition_interval = '1 day'::interval THEN
          l_part_name_3rd := format('%s_%s_%s', metric, dbname, to_char(l_part_start, 'yyyymmdd'));
      -- Check if interval is single week (1 week only)
      ELSIF l_partition_interval = '1 week'::interval THEN
          l_year := extract(isoyear from l_part_start);
          l_week := extract(week from l_part_start);
          l_part_name_3rd := format('%s_%s_y%sw%s', metric, dbname, l_year, to_char(l_week, 'fm00' ));
      -- Check if interval is single month (1 month only)
      ELSIF l_partition_interval = '1 month'::interval THEN
          l_part_name_3rd := format('%s_%s_%s', metric, dbname, to_char(l_part_start, 'yyyymm'));
      -- Check if interval is single year (1 year only)
      ELSIF l_partition_interval = '1 year'::interval THEN
          l_part_name_3rd := format('%s_%s_%s', metric, dbname, to_char(l_part_start, 'yyyy'));
      ELSE
          -- For all multi-intervals and hour-based intervals, use boundary-based naming
          l_part_name_3rd := format('%s_%s_%s_to_%s', 
              metric, 
              dbname, 
              to_char(l_part_start, 'yyyymmdd_hh24mi'), 
              to_char(l_part_end, 'yyyymmdd_hh24mi'));
      END IF;

      IF char_length(l_part_name_3rd) > MAX_IDENT_LEN     -- use "dbname" hash instead of name for overly long ones
      THEN
          -- Calculate ideal length based on the naming pattern used
          IF l_partition_interval = '1 day'::interval THEN
              ideal_length = MAX_IDENT_LEN - char_length(format('%s__%s', metric, to_char(l_part_start, 'yyyymmdd')));
              l_part_name_3rd := format('%s_%s_%s', metric, substring(md5(dbname) from 1 for ideal_length), to_char(l_part_start, 'yyyymmdd'));
          ELSIF l_partition_interval = '1 week'::interval THEN
              ideal_length = MAX_IDENT_LEN - char_length(format('%s__y%sw%s', metric, l_year, to_char(l_week, 'fm00')));
              l_part_name_3rd := format('%s_%s_y%sw%s', metric, substring(md5(dbname) from 1 for ideal_length), l_year, to_char(l_week, 'fm00' ));
          ELSIF l_partition_interval = '1 month'::interval THEN
              ideal_length = MAX_IDENT_LEN - char_length(format('%s__%s', metric, to_char(l_part_start, 'yyyymm')));
              l_part_name_3rd := format('%s_%s_%s', metric, substring(md5(dbname) from 1 for ideal_length), to_char(l_part_start, 'yyyymm'));
          ELSIF l_partition_interval = '1 year'::interval THEN
              ideal_length = MAX_IDENT_LEN - char_length(format('%s__%s', metric, to_char(l_part_start, 'yyyy')));
              l_part_name_3rd := format('%s_%s_%s', metric, substring(md5(dbname) from 1 for ideal_length), to_char(l_part_start, 'yyyy'));
          ELSE
              -- For all multi-intervals and hour-based intervals, use hash for dbname and truncate boundary timestamps if needed
              ideal_length = MAX_IDENT_LEN - char_length(format('%s__%s_to_%s', metric, to_char(l_part_start, 'yyyymmdd_hh24mi'), to_char(l_part_end, 'yyyymmdd_hh24mi')));
              IF ideal_length < 8 THEN
                  -- If still too long, use shorter timestamp format
                  l_part_name_3rd := format('%s_%s_%s_to_%s', 
                      metric, 
                      substring(md5(dbname) from 1 for 8), 
                      to_char(l_part_start, 'mmdd_hh24mi'), 
                      to_char(l_part_end, 'mmdd_hh24mi'));
              ELSE
                  l_part_name_3rd := format('%s_%s_%s_to_%s', 
                      metric, 
                      substring(md5(dbname) from 1 for ideal_length), 
                      to_char(l_part_start, 'yyyymmdd_hh24mi'), 
                      to_char(l_part_end, 'yyyymmdd_hh24mi'));
              END IF;
          END IF;
      END IF;

      IF NOT EXISTS (SELECT 1
                      FROM pg_tables
                      WHERE tablename = l_part_name_3rd
                        AND schemaname = 'subpartitions')
      THEN
        l_sql := format($$CREATE TABLE IF NOT EXISTS subpartitions.%I PARTITION OF subpartitions.%I FOR VALUES FROM ('%s') TO ('%s')$$,
                        l_part_name_3rd, l_part_name_2nd, l_part_start, l_part_end);
        EXECUTE l_sql;
        EXECUTE format('COMMENT ON TABLE subpartitions.%I IS $$pgwatch-generated-metric-dbname-time-lvl$$', l_part_name_3rd);
      END IF;

  END LOOP;
  
END;
$SQL$ LANGUAGE plpgsql;

-- GRANT EXECUTE ON FUNCTION admin.ensure_partition_metric_dbname_time(text,text,timestamp with time zone,integer) TO pgwatch;
