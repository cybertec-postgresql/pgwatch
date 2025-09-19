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
  MAX_IDENT_LEN CONSTANT integer := current_setting('max_identifier_length')::int;
BEGIN

  PERFORM pg_advisory_xact_lock(regexp_replace( md5(metric) , E'\\D', '', 'g')::varchar(10)::int8);

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

      l_year := extract(isoyear from (metric_timestamp + '1month'::interval * i));
      l_week := extract(week from (metric_timestamp + '1week'::interval * i));

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

      l_part_name_3rd := format('%s_%s_y%sw%s', metric, dbname, l_year, to_char(l_week, 'fm00' ));

      IF char_length(l_part_name_3rd) > MAX_IDENT_LEN     -- use "dbname" hash instead of name for overly long ones
      THEN
          ideal_length = MAX_IDENT_LEN - char_length(format('%s__y%sw%s', metric, l_year, to_char(l_week, 'fm00')));
          l_part_name_3rd := format('%s_%s_y%sw%s', metric, substring(md5(dbname) from 1 for ideal_length), l_year, to_char(l_week, 'fm00' ));
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
