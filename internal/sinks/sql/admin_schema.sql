/*
 "admin" schema - stores schema type, partition templates and data cleanup functions
 "public" schema - top level metric tables
 "subpartitions" schema - subpartitions of "public" schema top level metric tables (if using time / dbname-time partitioning)
*/

create schema "admin";
create schema "subpartitions";

create extension if not exists btree_gin;

create function admin.get_default_storage_type() returns text as
$$
 select case 
  when exists(select 1 from pg_extension where extname = 'timescaledb') then
    'timescale' 
  else 
    'postgres' 
  end;
$$
language sql;

create table admin.storage_schema_type (
  schema_type     text        not null default admin.get_default_storage_type(),
  initialized_on  timestamptz not null default now(),
  check (schema_type in ('postgres', 'timescale'))
);

insert into admin.storage_schema_type default values;

comment on table admin.storage_schema_type is 'identifies storage schema for other pgwatch components';

create unique index max_one_row on admin.storage_schema_type ((1));

/* for the Grafana drop-down. managed by the gatherer */
create table admin.all_distinct_dbname_metrics (
  dbname      text        not null,
  metric      text        not null,
  created_on  timestamptz not null default now(),
  primary key (dbname, metric)
);

/* currently only used to store TimescaleDB chunk interval */
create table admin.config (
    key               text        not null primary key,
    value             text        not null,
    created_on        timestamptz not null default now(),
    last_modified_on  timestamptz
);

-- to later change the value call the admin.change_timescale_chunk_interval(interval) function!
-- as changing the row directly will only be effective for completely new tables (metrics).
insert into admin.config select 'timescale_chunk_interval', '2 days';
insert into admin.config select 'timescale_compress_interval', '1 day';

create or replace function trg_config_modified() returns trigger
as $$
begin
  new.last_modified_on = now();
  return new;
end;
$$
language plpgsql;

create trigger config_modified before update on admin.config
for each row execute function trg_config_modified();

/*
  creates a top level metric table if not already existing (non-existing tables show ugly warnings in Grafana).
  expects the "metrics_template" table to exist.
*/
create or replace function admin.ensure_dummy_metrics_table(metric text) returns boolean as
$sql$
declare
  l_schema_type text;
begin
  select schema_type into l_schema_type from admin.storage_schema_type;

  if to_regclass(format('public.%I', metric)) is not null then
    return false;
  end if;

  if l_schema_type = 'postgres' then
    execute format('CREATE TABLE public.%I (LIKE admin.metrics_template INCLUDING INDEXES) PARTITION BY LIST (dbname)', metric);
  elsif l_schema_type = 'timescale' then
    perform admin.ensure_partition_timescale(metric);
  end if;

  execute format('COMMENT ON TABLE public.%I IS $$pgwatch-generated-metric-lvl$$', metric);

  return true;
end;
$sql$ language plpgsql;

create table admin.metrics_template (
  time      timestamptz not null default now(),
  dbname    text        not null,
  data      jsonb       not null,
  tag_data  jsonb,
  check (false)
);

comment on table admin.metrics_template IS 'used as a template for all new metric definitions';

create index on admin.metrics_template (dbname, time);
-- create index on admin.metrics_template using brin (dbname, time);  /* consider BRIN instead for large data amounts */
-- create index on admin.metrics_template using gin (tag_data) where tag_data notnull;
