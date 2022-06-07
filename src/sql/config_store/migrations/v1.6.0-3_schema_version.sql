begin;

set role to pgwatch3;

/* this will allow auto-rollout of schema changes for future 1.6+ releases */
create table schema_version (
        sv_tag text primary key,
        sv_created_on timestamptz not null default now()
);

insert into pgwatch3.schema_version (sv_tag) values ('1.6.0');

end;

reset role;
