ALTER TABLE pgwatch3.monitored_db
  ADD md_dbtype text NOT NULL DEFAULT 'postgres'
    CHECK (md_dbtype in ('postgres', 'pgbouncer'));
