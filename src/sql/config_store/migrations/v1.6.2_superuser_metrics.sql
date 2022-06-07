begin;

alter table pgwatch3.metric add m_sql_su text default '';

insert into pgwatch3.schema_version (sv_tag) values ('1.6.2');

commit;
