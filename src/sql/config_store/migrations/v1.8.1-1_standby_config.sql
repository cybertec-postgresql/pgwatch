begin;

alter table pgwatch3.monitored_db
    add md_preset_config_name_standby text references pgwatch3.preset_config(pc_name),
    add md_config_standby jsonb;

alter table pgwatch3.monitored_db
    add constraint preset_or_custom_config_standby check
    (not (md_preset_config_name_standby is not null and md_config_standby is not null));

insert into pgwatch3.schema_version (sv_tag) values ('1.8.1')
on conflict do nothing;

commit;
