/* 
  get_top_level_metric_tables() returns names of all top-level metric tables
*/
CREATE OR REPLACE FUNCTION admin.get_top_level_metric_tables(
    OUT table_name text
)
RETURNS SETOF text AS
$SQL$
  SELECT c.oid::regclass::text AS table_name
  FROM pg_class c 
  WHERE relkind in ('r', 'p') and c.relnamespace = 'public'::regnamespace
  AND EXISTS (SELECT 1 FROM pg_attribute WHERE attrelid = c.oid AND attname = 'time')
  AND pg_catalog.obj_description(c.oid, 'pg_class') = 'pgwatch-generated-metric-lvl'
  ORDER BY 1
$SQL$ LANGUAGE sql;

/*
  drop_all_metric_tables() drops all top-level metric tables (and all their partitions/chunks)
*/
CREATE OR REPLACE FUNCTION admin.drop_all_metric_tables()
RETURNS int AS
$SQL$
DECLARE
  r record;
  i int := 0;
BEGIN
  FOR r IN SELECT table_name FROM admin.get_top_level_metric_tables()
  LOOP
    RAISE NOTICE 'dropping %', r.table_name;
    EXECUTE 'DROP TABLE ' || r.table_name;
    i := i + 1;
  END LOOP;
  
  EXECUTE 'TRUNCATE admin.all_distinct_dbname_metrics';
  
  RETURN i;
END;
$SQL$ LANGUAGE plpgsql;

/*
  truncate_all_metric_tables() truncates all top-level metric tables
*/
CREATE OR REPLACE FUNCTION admin.truncate_all_metric_tables()
RETURNS int AS
$SQL$
DECLARE
  r record;
  i int := 0;
BEGIN
  FOR r IN SELECT table_name from admin.get_top_level_metric_tables()
  LOOP
    RAISE NOTICE 'truncating %', r.table_name;
    EXECUTE 'TRUNCATE TABLE ' || r.table_name;
    i := i + 1;
  END LOOP;
  
  EXECUTE 'truncate admin.all_distinct_dbname_metrics';
  
  RETURN i;
END;
$SQL$ LANGUAGE plpgsql;

/*
get_old_time_partitions() returns names of old time-based partitions/chunks from all metric tables
    older_than_days - how old partitions should be to be returned (e.g. 7 = older than 7 days)
    schema_type     - if empty, reads from admin.storage_schema_type, otherwise uses the given value (e.g. 'postgres' or 'timescale')
*/
CREATE OR REPLACE FUNCTION admin.get_old_time_partitions(older_than interval, schema_type text default '')
    RETURNS SETOF text AS
$SQL$
BEGIN
  IF schema_type = '' THEN
    SELECT st.schema_type INTO schema_type FROM admin.storage_schema_type st;
  END IF;
  CASE schema_type
  WHEN 'postgres' THEN
    RETURN QUERY
      SELECT
        c.oid::regclass::text AS time_partition_name
      FROM
        pg_class c JOIN pg_inherits i ON c.oid = i.inhrelid
      WHERE
        c.relkind IN ('r', 'p')
        AND c.relnamespace = 'subpartitions'::regnamespace
        AND (regexp_match(pg_catalog.pg_get_expr(c.relpartbound, c.oid), E'TO \\((''.*?'')'))[1]::timestamp < (now()  - older_than)
        AND pg_catalog.obj_description(c.oid, 'pg_class') IN ('pgwatch-generated-metric-time-lvl', 'pgwatch-generated-metric-dbname-time-lvl')
      ORDER BY 1;
  WHEN 'timescale' THEN
    RETURN QUERY
      SELECT chunk::text
      FROM timescaledb_information.hypertables, show_chunks(hypertable_name::regclass, older_than) as chunk
      ORDER BY 1;
  ELSE
    RAISE EXCEPTION 'unsupported schema type: %', schema_type;
  END CASE;
END;
$SQL$ LANGUAGE plpgsql;

/* 
drop_old_time_partitions() drops old time-based partitions/chunks from all metric tables
    older_than_days - how old partitions should be to be dropped (e.g. 7 = older than 7 days)
    schema_type     - if empty, reads from admin.storage_schema_type, otherwise uses the given value (e.g. 'postgres' or 'timescale')
*/
CREATE OR REPLACE FUNCTION admin.drop_old_time_partitions(older_than interval, schema_type text default '')
RETURNS int AS
$SQL$
DECLARE
  r record;
  r2 record;
  i int := 0;
  s text;
BEGIN
  IF schema_type = '' THEN
      SELECT st.schema_type INTO schema_type FROM admin.storage_schema_type st;
  END IF;
  CASE schema_type 
    WHEN 'postgres' THEN
      FOR r IN (
        SELECT 
          time_partition_name,
          c.oid::regclass AS parent_table_name
        FROM
          admin.get_old_time_partitions(older_than, 'postgres') AS p(time_partition_name)
          JOIN pg_inherits i ON p.time_partition_name::regclass = i.inhrelid
          JOIN pg_class c ON i.inhparent = c.oid
      ) LOOP
        RAISE NOTICE 'detaching sub-partition: %', r.time_partition_name;
        EXECUTE 'ALTER TABLE ' || r.parent_table_name || ' DETACH PARTITION ' || r.time_partition_name;
        RAISE NOTICE 'dropping sub-partition:  %', r.time_partition_name;
        EXECUTE 'DROP TABLE IF EXISTS ' || r.time_partition_name;
        i := i + 1;
      END LOOP;
    WHEN 'timescale' THEN
      SELECT count(*) INTO i
      FROM timescaledb_information.hypertables, drop_chunks(hypertable_name::regclass, older_than);
    ELSE
      RAISE EXCEPTION 'unsupported schema type: %', schema_type;
  END CASE;
  RETURN i;
END;
$SQL$ LANGUAGE plpgsql;

/*
maintain_unique_sources() maintains a mapping of unique sources in each metric table 
in admin.all_distinct_dbname_metrics. This is used to avoid listing the same source 
multiple times in Grafana dropdowns.

Returns the total number of rows affected by all operations.
*/
CREATE OR REPLACE FUNCTION admin.maintain_unique_sources()
RETURNS int AS
$SQL$
DECLARE
  r record;
  v_found_dbnames text[];
  v_all_metrics text[] := ARRAY[]::text[];
  v_rows_affected bigint;
  v_total_rows_affected bigint := 0;
  v_metric_name text;
BEGIN
  -- Try to get advisory lock (1571543679778230000 is a random bigint)
  -- This ensures only one instance runs maintenance at a time
  -- Using xact variant so lock is automatically released at transaction end
  IF NOT pg_try_advisory_xact_lock(1571543679778230000) THEN
    RAISE NOTICE 'Skipping admin.all_distinct_dbname_metrics maintenance as another instance has the advisory lock';
    RETURN 0;
  END IF;
  
  RAISE NOTICE 'Refreshing admin.all_distinct_dbname_metrics listing table';
  
  -- Get all top-level metric tables
  FOR r IN SELECT table_name FROM admin.get_top_level_metric_tables()
  LOOP
    v_metric_name := replace(r.table_name, 'public.', '');
    v_all_metrics := array_append(v_all_metrics, v_metric_name);
    
    RAISE DEBUG 'Refreshing all_distinct_dbname_metrics listing for metric: %', v_metric_name;
    
    -- Get distinct dbnames from the metric table using recursive CTE
    -- This is more efficient than DISTINCT for large tables
    EXECUTE format($f$
      WITH RECURSIVE t(dbname) AS (
        SELECT MIN(dbname) AS dbname FROM %I
        UNION
        SELECT (SELECT MIN(dbname) FROM %I WHERE dbname > t.dbname) FROM t 
      )
      SELECT array_agg(dbname) FROM t WHERE dbname IS NOT NULL
    $f$, r.table_name, r.table_name) INTO v_found_dbnames;
    
    -- Handle case where no dbnames found - delete all entries for this metric
    IF v_found_dbnames IS NULL THEN
      WITH deleted AS (
        DELETE FROM admin.all_distinct_dbname_metrics 
        WHERE metric = v_metric_name
        RETURNING 1
      )
      SELECT count(*) INTO v_rows_affected FROM deleted;
      
      IF v_rows_affected > 0 THEN
        RAISE DEBUG 'Deleted all admin.all_distinct_dbname_metrics table entries for metric: %', v_metric_name;
        v_total_rows_affected := v_total_rows_affected + v_rows_affected;
      END IF;
      
      CONTINUE;
    END IF;
    
    -- Delete stale entries (dbnames that no longer exist in the metric table)
    WITH deleted AS (
      DELETE FROM admin.all_distinct_dbname_metrics 
      WHERE NOT dbname = ANY(v_found_dbnames) 
        AND metric = v_metric_name
      RETURNING 1
    )
    SELECT count(*) INTO v_rows_affected FROM deleted;
    
    IF v_rows_affected > 0 THEN
      RAISE NOTICE 'Removed % stale entries from admin.all_distinct_dbname_metrics table for metric: %', v_rows_affected, v_metric_name;
      v_total_rows_affected := v_total_rows_affected + v_rows_affected;
    END IF;
    
    -- Add new entries (dbnames that exist in metric table but not in listing)
    WITH inserted AS (
      INSERT INTO admin.all_distinct_dbname_metrics (dbname, metric)
      SELECT unnest(v_found_dbnames), v_metric_name
      WHERE NOT EXISTS (
        SELECT 1 FROM admin.all_distinct_dbname_metrics 
        WHERE dbname = ANY(v_found_dbnames) 
          AND metric = v_metric_name
      )
      RETURNING 1
    )
    SELECT count(*) INTO v_rows_affected FROM inserted;
    
    IF v_rows_affected > 0 THEN
      RAISE NOTICE 'Added % entries to admin.all_distinct_dbname_metrics table for metric: %', v_rows_affected, v_metric_name;
      v_total_rows_affected := v_total_rows_affected + v_rows_affected;
    END IF;
  END LOOP;
  
  -- Delete entries for dropped metric tables
  WITH deleted AS (
    DELETE FROM admin.all_distinct_dbname_metrics 
    WHERE metric != ALL(v_all_metrics)
    RETURNING 1
  )
  SELECT count(*) INTO v_rows_affected FROM deleted;
  
  IF v_rows_affected > 0 THEN
    RAISE NOTICE 'Removed % stale entries for dropped tables from admin.all_distinct_dbname_metrics table', v_rows_affected;
    v_total_rows_affected := v_total_rows_affected + v_rows_affected;
  END IF;
  
  RETURN v_total_rows_affected;
END;
$SQL$ LANGUAGE plpgsql;
