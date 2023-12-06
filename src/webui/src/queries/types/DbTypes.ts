export type Db = {
  md_name: string;
  md_connstr: string;
  md_is_superuser: boolean;
  md_preset_config_name: string;
  md_config: string | null;
  md_is_enabled: boolean;
  md_last_modified_on: string;
  md_dbtype: string;
  md_include_pattern: string | null;
  md_exclude_pattern: string | null;
  md_custom_tags: string | null;
  md_group: string;
  md_encryption: string;
  md_host_config: string | null;
  md_only_if_master: boolean;
  md_preset_config_name_standby: string | null;
  md_config_standby: string | null;
}

export type createDbForm = {
  md_name: string;
  md_connstr: string;
  md_is_superuser: boolean;
  md_preset_config_name: string;
  md_config: string | null;
  md_is_enabled: boolean;
  md_last_modified_on: string;
  md_dbtype: string;
  md_include_pattern: string | null;
  md_exclude_pattern: string | null;
  md_custom_tags: string | null;
  md_group: string;
  md_encryption: string;
  md_host_config: string | null;
  md_only_if_master: boolean;
  md_preset_config_name_standby: string | null;
  md_config_standby: string | null;
};

export type updateDbForm = {
  md_unique_name: string,
  data: createDbForm
};

export type updateEnabledDbForm = {
  md_unique_name: string,
  data: {
    md_is_enabled: boolean
  }
};
