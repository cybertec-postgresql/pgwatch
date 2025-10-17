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
  -- Validate the interval - allow standard intervals or custom intervals
  IF new_interval NOT IN ('1 day'::interval, '1 week'::interval, '1 month'::interval) THEN
    -- For custom intervals, validate they are reasonable (between 1 hour and 1 month)
    IF new_interval < '1 hour'::interval OR new_interval > '1 month'::interval THEN
      RAISE EXCEPTION 'Custom partition interval must be between 1 hour and 1 month. Got: %', new_interval;
    END IF;
    
    -- Prohibit minute and second-based intervals
    IF new_interval < '1 hour'::interval THEN
      RAISE EXCEPTION 'Minute and second-based intervals are not allowed. Use hours, days, weeks, or months instead. Got: %', new_interval;
    END IF;
  END IF;
  
  -- Prohibit 1 year intervals
  IF new_interval = '1 year'::interval THEN
    RAISE EXCEPTION '1 year partition intervals are not allowed. Use hours, days, weeks, or months instead. Got: %', new_interval;
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
