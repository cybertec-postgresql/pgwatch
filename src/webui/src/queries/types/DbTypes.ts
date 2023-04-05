export type Db = {
  md_id: number,
  md_port: string,
  md_user: string,
  md_group: string,
  md_config: string | null,
  md_dbname: string,
  md_dbtype: string,
  md_sslmode: string,
  md_hostname: string,
  md_password: string,
  md_is_enabled: boolean,
  md_custom_tags: string | null,
  md_host_config: string | null,
  md_unique_name: string,
  md_is_superuser: boolean,
  md_root_ca_path: string,
  md_password_type: string,
  md_config_standby: string | null,
  md_only_if_master: boolean,
  md_client_key_path: string,
  md_exclude_pattern: string | null,
  md_include_pattern: string | null,
  md_client_cert_path: string,
  md_last_modified_on: string,
  md_preset_config_name: string,
  md_statement_timeout_seconds: number,
  md_preset_config_name_standby: string | null
};

export type createDbForm = {
  md_unique_name: string,
  md_dbtype: string,
  md_group: string,
  md_custom_tags: string | null,
  md_password_type: string,
  md_is_enabled: boolean,
  md_is_superuser: boolean,
  md_hostname: string,
  md_port: string,
  md_dbname: string,
  md_include_pattern: string | null,
  md_exclude_pattern: string | null,
  md_user: string,
  md_password: string,
  md_statement_timeout_seconds: number,
  connection_timeout_seconds: number,
  md_sslmode: string,
  md_root_ca_path: string,
  md_client_cert_path: string,
  md_client_key_path: string,
  md_preset_config_name: string,
  md_config: string | null,
  md_preset_config_name_standby: string | null,
  md_config_standby: string | null,
  md_host_config: string | null,
  md_is_helpers: boolean,
  md_only_if_master: boolean
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
