-- DROP FUNCTION IF EXISTS admin.change_postgres_partition_interval(interval);
-- select * from admin.change_postgres_partition_interval('1 day');

CREATE OR REPLACE FUNCTION admin.change_postgres_partition_interval(
    new_interval interval
)
RETURNS void AS
/*
  changes the default partition interval for PostgreSQL native partitioning.
  writes the new default into the admin.config table so that future new metric tables would also automatically use it.
  
  Note: This only affects NEW partitions created after this change.
  Existing partitions will keep their current interval until they are dropped.
*/
$SQL$
BEGIN
  -- Validate the interval - only allow 1 day, 1 week, or 1 month
  IF new_interval NOT IN ('1 day'::interval, '1 week'::interval, '1 month'::interval) THEN
    RAISE EXCEPTION 'Partition interval must be exactly 1 day, 1 week, or 1 month. Got: %', new_interval;
  END IF;
  
  -- Update the configuration
  INSERT INTO admin.config
  SELECT 'postgres_partition_interval', new_interval::text
  ON CONFLICT (key) DO UPDATE
    SET value = new_interval::text;
    
  RAISE NOTICE 'PostgreSQL partition interval changed to: %. This will affect new partitions only.', new_interval;

END;
$SQL$ LANGUAGE plpgsql;

-- GRANT EXECUTE ON FUNCTION admin.change_postgres_partition_interval(interval) TO pgwatch;
