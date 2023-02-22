export type Metric = {
  m_id: number,
  m_sql: string,
  m_name: string,
  m_sql_su: string,
  m_comment: string | null,
  m_is_active: boolean,
  m_is_helper: boolean,
  m_master_only: boolean,
  m_column_attrs: Object,
  m_standby_only: boolean,
  m_pg_version_from: number,
  m_last_modified_on: string
};

export type createMetricForm = {
  m_name: string,
  m_pg_version_from: number,
  m_sql: string,
  m_comment: string | null,
  m_is_active: boolean,
  m_is_helper: boolean,
  m_master_only: boolean,
  m_standby_only: boolean,
  m_column_attrs: string | null,
  m_sql_su: string
};

export type updateMetricForm = {
  m_id: number,
  data: createMetricForm
};
