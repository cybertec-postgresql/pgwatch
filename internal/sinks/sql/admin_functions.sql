-- DROP FUNCTION IF EXISTS admin.get_top_level_metric_tables();
-- select * from admin.get_top_level_metric_tables();
CREATE OR REPLACE FUNCTION admin.get_top_level_metric_tables(
    OUT table_name text
)
RETURNS SETOF text AS
$SQL$
  select nspname||'.'||quote_ident(c.relname) as tbl
  from pg_class c 
  join pg_namespace n on n.oid = c.relnamespace
  where relkind in ('r', 'p') and nspname = 'public'
  and exists (select 1 from pg_attribute where attrelid = c.oid and attname = 'time')
  and pg_catalog.obj_description(c.oid, 'pg_class') = 'pgwatch-generated-metric-lvl'
  order by 1
$SQL$ LANGUAGE sql;

-- GRANT EXECUTE ON FUNCTION admin.get_top_level_metric_tables() TO pgwatch;


-- DROP FUNCTION IF EXISTS admin.drop_all_metric_tables();
-- select * from admin.drop_all_metric_tables();
CREATE OR REPLACE FUNCTION admin.drop_all_metric_tables()
RETURNS int AS
$SQL$
DECLARE
  r record;
  i int := 0;
BEGIN
  FOR r IN select * from admin.get_top_level_metric_tables()
  LOOP
    raise notice 'dropping %', r.table_name;
    EXECUTE 'DROP TABLE ' || r.table_name;
    i := i + 1;
  END LOOP;
  
  EXECUTE 'truncate admin.all_distinct_dbname_metrics';
  
  RETURN i;
END;
$SQL$ LANGUAGE plpgsql;

-- GRANT EXECUTE ON FUNCTION admin.drop_all_metric_tables() TO pgwatch;


-- DROP FUNCTION IF EXISTS admin.truncate_all_metric_tables();
-- select * from admin.truncate_all_metric_tables();
CREATE OR REPLACE FUNCTION admin.truncate_all_metric_tables()
RETURNS int AS
$SQL$
DECLARE
  r record;
  i int := 0;
BEGIN
  FOR r IN select * from admin.get_top_level_metric_tables()
  LOOP
    raise notice 'truncating %', r.table_name;
    EXECUTE 'TRUNCATE TABLE ' || r.table_name;
    i := i + 1;
  END LOOP;
  
  EXECUTE 'truncate admin.all_distinct_dbname_metrics';
  
  RETURN i;
END;
$SQL$ LANGUAGE plpgsql;

-- GRANT EXECUTE ON FUNCTION admin.truncate_all_metric_tables() TO pgwatch;


-- DROP FUNCTION IF EXISTS admin.remove_single_dbname_data(text);
-- select * from admin.remove_single_dbname_data('adhoc-1');
CREATE OR REPLACE FUNCTION admin.remove_single_dbname_data(dbname text)
RETURNS int AS
$SQL$
DECLARE
  r record;
  i int := 0;
  j int;
  l_schema_type text;
BEGIN
  SELECT schema_type INTO l_schema_type FROM admin.storage_schema_type;
  
  IF l_schema_type = 'timescale' THEN
    FOR r IN select * from admin.get_top_level_metric_tables()
    LOOP
      raise notice 'deleting data for %', r.table_name;
      EXECUTE format('DELETE FROM %s WHERE dbname = $1', r.table_name) USING dbname;
      GET DIAGNOSTICS j = ROW_COUNT;
      i := i + j;
    END LOOP;
  ELSIF l_schema_type = 'postgres' THEN
    FOR r IN (
            select 'subpartitions.'|| quote_ident(c.relname) as table_name
                 from pg_class c
                join pg_namespace n on n.oid = c.relnamespace
                join pg_inherits i ON c.oid=i.inhrelid                
                join pg_class c2 on i.inhparent = c2.oid
                where c.relkind in ('r', 'p') and nspname = 'subpartitions'
                and exists (select 1 from pg_attribute where attrelid = c.oid and attname = 'time')
                and pg_catalog.obj_description(c.oid, 'pg_class') = 'pgwatch-generated-metric-dbname-lvl'
                and (regexp_match(pg_catalog.pg_get_expr(c.relpartbound, c.oid), E'FOR VALUES IN \\(''(.*)''\\)'))[1] = dbname
                order by 1
    )
    LOOP
        raise notice 'dropping sub-partition % ...', r.table_name;
        EXECUTE 'drop table ' || r.table_name;
        GET DIAGNOSTICS j = ROW_COUNT;
        i := i + j;
    END LOOP;
  ELSE
    raise exception 'unsupported schema type: %', l_schema_type;
  END IF;
  
  EXECUTE 'delete from admin.all_distinct_dbname_metrics where dbname = $1' USING dbname;
  
  RETURN i;
END;
$SQL$ LANGUAGE plpgsql;

-- GRANT EXECUTE ON FUNCTION admin.remove_single_dbname_data(text) TO pgwatch;


-- drop function if exists admin.drop_old_time_partitions(int,bool)
-- select * from admin.drop_old_time_partitions(1, true);
-- Note: This function uses a two-phase approach: DETACH PARTITION first, then DROP TABLE for safer partition removal
CREATE OR REPLACE FUNCTION admin.drop_old_time_partitions(older_than_days int, dry_run boolean default true, schema_type text default '')
RETURNS int AS
$SQL$
DECLARE
  r record;
  r2 record;
  i int := 0;
BEGIN

  IF schema_type = '' THEN
    SELECT st.schema_type INTO schema_type FROM admin.storage_schema_type st;
  END IF;


  IF schema_type = 'timescale' THEN

        if dry_run then
            -- Loop over all top-level hypertables
            FOR r IN (
                select
                  h.table_name::text as metric
                from
                  _timescaledb_catalog.hypertable h
                where
                  h.schema_name = 'public'
            )
            LOOP
                -- Get old chunks for this hypertable using show_chunks()
                FOR r2 IN (
                    SELECT show_chunks(r.metric, older_than => (older_than_days * '1 day'::interval)) as chunk_name
                )
                LOOP
                    raise notice 'would execute: CALL detach_chunk(%)', r2.chunk_name;
                    raise notice 'would execute: DROP TABLE %', r2.chunk_name;
                END LOOP;
            END LOOP;

        else /* loop over all to level hypertables */
            FOR r IN (
                select
                  h.table_name::text as metric
                from
                  _timescaledb_catalog.hypertable h
                where
                  h.schema_name = 'public'
            )
            LOOP
                -- Get old chunks for this hypertable using show_chunks()
                FOR r2 IN (
                    SELECT show_chunks(r.metric, older_than => (older_than_days * '1 day'::interval)) as chunk_name
                )
                LOOP
                    raise notice 'detaching old timescale chunk: % from hypertable: %', r2.chunk_name, r.metric;
                    
                    -- Detach the chunk using TimescaleDB's detach_chunk procedure
                    BEGIN
                        EXECUTE 'CALL detach_chunk(' || quote_literal(r2.chunk_name) || ')';
                        raise notice 'successfully detached chunk: %', r2.chunk_name;
                    EXCEPTION
                        WHEN OTHERS THEN
                            raise notice 'failed to detach chunk %: %', r2.chunk_name, SQLERRM;
                            CONTINUE;
                    END;
                    
                    raise notice 'dropping detached chunk: %', r2.chunk_name;
                    -- Drop the detached chunk table using DROP TABLE
                    BEGIN
                        EXECUTE 'DROP TABLE ' || r2.chunk_name::text;
                        raise notice 'successfully dropped chunk: %', r2.chunk_name;
                    EXCEPTION
                        WHEN OTHERS THEN
                            raise notice 'failed to drop chunk %: %', r2.chunk_name, SQLERRM;
                            CONTINUE;
                    END;
                    
                    i := i + 1;
                END LOOP;
            END LOOP;
        end if;

  ELSE
    raise warning 'unsupported schema type: %', l_schema_type;
  END IF;

  RETURN i;
END;
$SQL$ LANGUAGE plpgsql;

-- GRANT EXECUTE ON FUNCTION admin.drop_old_time_partitions(int,bool,text) TO pgwatch;

-- drop function if exists admin.get_old_time_partitions(int,text);
-- select * from admin.get_old_time_partitions(1);
CREATE OR REPLACE FUNCTION admin.get_old_time_partitions(older_than_days int, schema_type text default '')
    RETURNS SETOF text AS
$SQL$
BEGIN

    IF schema_type = '' THEN
        SELECT st.schema_type INTO schema_type FROM admin.storage_schema_type st;
    END IF;

    IF schema_type = 'postgres' THEN

        RETURN QUERY
            SELECT time_partition_name FROM (
                SELECT
                    'subpartitions.' || quote_ident(c.relname) as time_partition_name,
                    pg_catalog.pg_get_expr(c.relpartbound, c.oid) as limits,
                    (regexp_match(pg_catalog.pg_get_expr(c.relpartbound, c.oid),
                        E'TO \\((''.*?'')'))[1]::timestamp < (
                            current_date  - '1day'::interval * older_than_days
                        ) is_old
                FROM
                    pg_class c
                        JOIN
                    pg_inherits i ON c.oid=i.inhrelid
                        JOIN
                    pg_namespace n ON n.oid = relnamespace
                WHERE
                        c.relkind IN ('r', 'p')
                  AND nspname = 'subpartitions'
                  AND pg_catalog.obj_description(c.oid, 'pg_class') IN (
                        'pgwatch-generated-metric-time-lvl',
                        'pgwatch-generated-metric-dbname-time-lvl'
                    )
            ) x
            WHERE is_old
            ORDER BY 1;
    ELSE
        RAISE EXCEPTION 'only postgres partitioning schemas supported currently!';
    END IF;

END;
$SQL$ LANGUAGE plpgsql;

-- GRANT EXECUTE ON FUNCTION admin.get_old_time_partitions(int,text) TO pgwatch;
