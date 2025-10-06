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
