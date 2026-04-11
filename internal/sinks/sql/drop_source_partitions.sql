/*
  drop_source_partitions(p_source_name) removes all metric data for a decommissioned source.

  It finds every Level-2 (dbname) partition across all metrics by inspecting the partition
  bound expression in pg_catalog rather than reconstructing table names. This is necessary
  because ensure_partition_metric_dbname_time uses an MD5 hash fallback when the concatenated
  name exceeds max_identifier_length, making name-based lookups unreliable.

  For each matching partition it:
    1. Acquires the same per-metric advisory lock used by ensure_partition_metric_dbname_time
       to prevent concurrent partition creation/deletion races.
    2. Detaches the Level-2 partition from its parent metric table.
    3. Drops the Level-2 partition (which cascades to all Level-3 time partitions).
    4. Cleans up the admin.all_distinct_dbname_metrics Grafana helper table.

  The function is idempotent: calling it for an already-removed source is a safe no-op.

  Parameters:
    p_source_name - the dbname (source) whose partitions should be dropped

  Returns:
    the number of Level-2 partitions dropped
*/
CREATE OR REPLACE FUNCTION admin.drop_source_partitions(p_source_name text)
RETURNS int AS
$SQL$
DECLARE
  r record;
  v_dropped_count int := 0;
BEGIN
  IF p_source_name IS NULL OR p_source_name = '' THEN
    RAISE EXCEPTION 'p_source_name must be a non-empty string';
  END IF;

  /*
    Find every Level-2 partition belonging to p_source_name.

    - pg_get_expr(c.relpartbound, c.oid) returns something like:  FOR VALUES IN ('mydb')
    - We extract the value inside the single quotes with a regex.
    - The comment tag 'pgwatch-generated-metric-dbname-lvl' guarantees we only touch
      pgwatch-managed tables, never user-created ones.
    - parent.relname gives us the Level-1 metric table name (e.g. 'cpu_load'),
      which we need for DETACH and for the advisory lock.
  */
  FOR r IN
    SELECT
      c.relname                  AS partition_name,
      parent.relname             AS metric_name
    FROM pg_catalog.pg_class c
    JOIN pg_catalog.pg_inherits i      ON c.oid = i.inhrelid
    JOIN pg_catalog.pg_class parent    ON parent.oid = i.inhparent
    WHERE c.relnamespace = 'subpartitions'::regnamespace
      AND pg_catalog.obj_description(c.oid, 'pg_class') = 'pgwatch-generated-metric-dbname-lvl'
      AND (regexp_match(
             pg_catalog.pg_get_expr(c.relpartbound, c.oid),
             E'FOR VALUES IN \\(''(.+?)''\\)'
           ))[1] = p_source_name
  LOOP
    /*
      Acquire the same advisory lock that ensure_partition_metric_dbname_time takes.
      The lock key is derived from md5(metric_name) with all non-digit characters
      stripped, then truncated to fit into a bigint. This serialises against the
      gatherer creating new partitions for the same metric while we drop.
    */
    PERFORM pg_advisory_xact_lock(
      regexp_replace(md5(r.metric_name), E'\\D', '', 'g')::varchar(10)::int8
    );

    RAISE NOTICE 'detaching partition: subpartitions.% from public.%', r.partition_name, r.metric_name;
    EXECUTE format(
      'ALTER TABLE public.%I DETACH PARTITION subpartitions.%I',
      r.metric_name, r.partition_name
    );

    RAISE NOTICE 'dropping partition:  subpartitions.%', r.partition_name;
    EXECUTE format(
      'DROP TABLE IF EXISTS subpartitions.%I CASCADE',
      r.partition_name
    );

    v_dropped_count := v_dropped_count + 1;
  END LOOP;

  DELETE FROM admin.all_distinct_dbname_metrics WHERE dbname = p_source_name;

  IF v_dropped_count > 0 THEN
    RAISE NOTICE 'dropped % partition(s) and cleaned up metadata for source: %', v_dropped_count, p_source_name;
  ELSE
    RAISE NOTICE 'no partitions found for source: % (already clean or never existed)', p_source_name;
  END IF;

  RETURN v_dropped_count;
END;
$SQL$ LANGUAGE plpgsql;
