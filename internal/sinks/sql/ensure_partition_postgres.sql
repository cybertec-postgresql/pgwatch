/*
  ensure_partition_metric_time creates partitioned metric tables if not already existing
    metric - name of the metric (top level table)
    metric_timestamp - timestamp of the metric (used to determine the time partition)
    partition_period - interval for partitioning (e.g., '1 week', '1 day', '1 month')
    partitions_to_precreate - how many future time partitions to create (default 3)
    part_available_from - output parameter, start time of the time partition where the given metric_timestamp fits in
    part_available_to - output parameter, end time of the time partition where the given metric_timestamp fits in
*/
CREATE OR REPLACE FUNCTION admin.ensure_partition_metric_time(
    metric text,
    metric_timestamp timestamptz,
    partition_period interval default '1 week'::interval,
    partitions_to_precreate int default 3,
    OUT part_available_from timestamptz,
    OUT part_available_to timestamptz)
RETURNS record AS
/*
  creates a top level metric table and time partitions if not already existing.
  returns time partition start/end date
*/
$SQL$
DECLARE
  l_part_name text;
  l_part_start timestamptz;
  l_part_end timestamptz;
  ideal_length int;
  l_template_table text := 'admin.metrics_template';
  l_partition_format text;
  l_time_suffix text;
  l_existing_upper_bound timestamptz;
BEGIN
  -- Validate partition period
  IF partition_period < interval '1 hour' THEN
    RAISE EXCEPTION 'Partition period must be at least 1 hour, got: %', partition_period;
  END IF;

  -- Determine partition naming format based on period
  CASE
    WHEN partition_period >= interval '1 day' THEN
      l_partition_format := 'YYYYMMDD';
    ELSE
      -- For hourly partitions (>= 1 hour, < 1 day)
      l_partition_format := 'YYYYMMDD_HH24';
  END CASE;

  PERFORM pg_advisory_xact_lock(regexp_replace( md5(metric) , E'\\D', '', 'g')::varchar(10)::int8);

  -- Check if table exists and has LIST partitioning (legacy); if so, drop it for conversion to RANGE
  IF EXISTS 
      (
        SELECT 1 
        FROM pg_partitioned_table 
        WHERE 
          partrelid = to_regclass('public.' || quote_ident(metric))
          AND
          partstrat = 'l'
      ) THEN
    EXECUTE format('DROP TABLE public.%I CASCADE', metric);
  END IF;

  -- 1. level
  IF to_regclass('public.' || quote_ident(metric)) IS NULL
  THEN
    EXECUTE format('CREATE TABLE public.%I (LIKE admin.metrics_template INCLUDING INDEXES) PARTITION BY RANGE (time)', metric);
    EXECUTE format('COMMENT ON TABLE public.%I IS $$pgwatch-generated-metric-lvl$$', metric);
  END IF;

  -- 2. level

  -- Get existing partition upper bound
  SELECT max(substring(pg_catalog.pg_get_expr(c.relpartbound, c.oid, true) from 'TO \(''([^'']+)''')::timestamptz)
  INTO l_existing_upper_bound
  FROM pg_catalog.pg_class c
  JOIN pg_catalog.pg_inherits i ON i.inhrelid = c.oid
  JOIN pg_catalog.pg_class parent ON parent.oid = i.inhparent
  WHERE c.relispartition
    AND c.relnamespace = 'subpartitions'::regnamespace
    AND parent.relname = metric;

  -- Determine starting point for new partitions
  IF l_existing_upper_bound IS NOT NULL THEN
    -- Start from the existing upper bound to maintain continuity
    l_part_start := l_existing_upper_bound;
  ELSE
    -- No existing partitions, align to clean boundaries based on period size
    CASE
      WHEN partition_period >= interval '1 week' THEN
        l_part_start := date_trunc('week', metric_timestamp);
      WHEN partition_period >= interval '1 day' THEN
        l_part_start := date_trunc('day', metric_timestamp);
      ELSE
        -- For hourly periods (>= 1 hour, < 1 day)
        l_part_start := date_trunc('hour', metric_timestamp);
    END CASE;

    -- For the first partition, set the available range
    part_available_from := l_part_start;
    part_available_to := l_part_start + partition_period;
  END IF;

  -- Create partitions
  FOR i IN 0..partitions_to_precreate LOOP
      l_part_end := l_part_start + partition_period;

      -- Update the available range for the first partition only if we started from metric_timestamp
      IF i = 0 AND l_existing_upper_bound IS NULL THEN
          part_available_from := l_part_start;
          part_available_to := l_part_end;
      ELSIF i = 0 AND l_existing_upper_bound IS NOT NULL THEN
          -- For existing partitions, we need to find which partition contains the metric_timestamp
          IF metric_timestamp >= l_part_start AND metric_timestamp < l_part_end THEN
              part_available_from := l_part_start;
              part_available_to := l_part_end;
          ELSEIF metric_timestamp < l_part_start THEN
              EXIT; -- No need to create further partitions
          END IF;
      END IF;

      l_time_suffix := to_char(l_part_start, l_partition_format);
      l_part_name := format('%s_%s', metric, l_time_suffix);

      IF to_regclass('subpartitions.' || quote_ident(l_part_name)) IS NULL
      THEN
        EXECUTE format('CREATE TABLE subpartitions.%I PARTITION OF public.%I FOR VALUES FROM ($$%s$$) TO ($$%s$$)',
                        l_part_name, metric, l_part_start, l_part_end);
        EXECUTE format('COMMENT ON TABLE subpartitions.%I IS $$pgwatch-generated-metric-time-lvl$$', l_part_name);
      END IF;

      l_part_start := l_part_end;
  END LOOP;

  -- If we still don't have part_available_from/to set, find the partition containing metric_timestamp
  IF part_available_from IS NULL THEN
    SELECT lower_text::timestamptz, upper_text::timestamptz
    INTO part_available_from, part_available_to
    FROM (
      SELECT substring(pg_catalog.pg_get_expr(c.relpartbound, c.oid, true) from 'FOR VALUES FROM \(''([^'']+)''') AS lower_text,
             substring(pg_catalog.pg_get_expr(c.relpartbound, c.oid, true) from 'TO \(''([^'']+)''') AS upper_text
      FROM pg_catalog.pg_class c
      JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
      JOIN pg_catalog.pg_inherits i ON i.inhrelid = c.oid
      JOIN pg_catalog.pg_class parent ON parent.oid = i.inhparent
      WHERE c.relispartition
        AND n.nspname = 'subpartitions'
        AND parent.relname = metric
    ) AS partitions
    WHERE metric_timestamp >= lower_text::timestamptz
      AND metric_timestamp < upper_text::timestamptz
    LIMIT 1;
  END IF;

END;
$SQL$ LANGUAGE plpgsql;
