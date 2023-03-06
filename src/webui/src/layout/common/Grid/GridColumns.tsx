import { Box, Checkbox } from "@mui/material";
import { GridColDef, GridRenderCellParams } from "@mui/x-data-grid";

import { UseMutationResult } from "@tanstack/react-query";

import moment from "moment";

import { Db } from "queries/types/DbTypes";
import { Metric } from "queries/types/MetricTypes";
import { GridActionsComponent } from "./GridActionsComponent";

type metricsColumnsProps = {
  setEditData: React.Dispatch<React.SetStateAction<Metric | undefined>>,
  handleModalOpen: () => void,
  deleteRecord: UseMutationResult<any, any, any, unknown>
}

export const metricsColumns = ({
  setEditData,
  handleModalOpen,
  deleteRecord
}: metricsColumnsProps): GridColDef[] => {
  return [
    {
      field: "m_id",
      headerName: "ID",
      width: 75,
      type: "number",
      align: "center",
      headerAlign: "center",
      hide: true
    },
    {
      field: "m_name",
      headerName: "Metric name",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "m_pg_version_from",
      headerName: "PG version",
      width: 150,
      type: "number",
      align: "center",
      headerAlign: "center"
    },
    {
      field: "m_comment",
      headerName: "Comment",
      width: 200,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "m_is_active",
      headerName: "Enabled?",
      width: 120,
      renderCell: (params: GridRenderCellParams<boolean>) => <Checkbox checked={params.value} disableRipple />,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "m_is_helper",
      headerName: "Helpers?",
      width: 120,
      renderCell: (params: GridRenderCellParams<boolean>) => <Checkbox checked={params.value} disableRipple />,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "m_master_only",
      headerName: "Master only?",
      width: 120,
      renderCell: (params: GridRenderCellParams<boolean>) => <Checkbox checked={params.value} disableRipple />,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "m_standby_only",
      headerName: "Standby only?",
      width: 120,
      renderCell: (params: GridRenderCellParams<boolean>) => <Checkbox checked={params.value} disableRipple />,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "m_last_modified_on",
      headerName: "Last modified",
      width: 170,
      renderCell: (params) =>
        <Box sx={{ display: "flex", alignItems: "center", textAlign: "center" }}>
          {moment(params.value).format("DD.MM.YYYY HH:mm:ss")}
        </Box>,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "m_actions",
      headerName: "Actions",
      type: "actions",
      width: 200,
      renderCell: (params) => (
        <GridActionsComponent
          data={params.row}
          setEditData={setEditData}
          handleModalOpen={handleModalOpen}
          deleteRecord={deleteRecord}
          deleteParameter={params.row.m_id}
          warningMessage={`Are you sure you want to delete metric named "${params.row.m_name}", version: ${params.row.m_pg_version_from}`}
        />
      ),
      headerAlign: "center"
    }
  ];
};

type databasesColumnsProps = {
  setEditData: React.Dispatch<React.SetStateAction<Db | undefined>>,
  handleModalOpen: () => void,
  deleteRecord: UseMutationResult<any, any, any, unknown>
};

export const databasesColumns = ({
  setEditData,
  handleModalOpen,
  deleteRecord
}: databasesColumnsProps): GridColDef[] => {
  return [
    {
      field: "md_id",
      headerName: "ID",
      width: 75,
      type: "number",
      align: "center",
      headerAlign: "center",
      hide: true
    },
    {
      field: "md_unique_name",
      headerName: "Unique name",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "md_dbtype",
      headerName: "DB type",
      width: 150,
      align: "center",
      headerAlign: "center",
      hide: true
    },
    {
      field: "md_connection",
      headerName: "Connection",
      width: 150,
      align: "center",
      headerAlign: "center",
      valueGetter(params) {
        return (`${params.row.md_hostname}:${params.row.md_port}`);
      },
    },
    {
      field: "md_dbname",
      headerName: "DB dbname",
      width: 150,
      align: "center",
      headerAlign: "center",
      hide: true
    },
    {
      field: "md_include_pattern",
      headerName: "DB name inclusion pattern",
      width: 150,
      align: "center",
      headerAlign: "center",
      hide: true
    },
    {
      field: "md_exclude_pattern",
      headerName: "DB name exclusion pattern",
      width: 150,
      align: "center",
      headerAlign: "center",
      hide: true
    },
    {
      field: "md_user",
      headerName: "DB user",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "md_is_superuser",
      headerName: "Super user?",
      width: 120,
      type: "boolean",
      renderCell: (params: GridRenderCellParams<boolean>) => <Checkbox checked={params.value} disableRipple />,
      align: "center",
      headerAlign: "center",
      hide: true
    },
    {
      field: "md_password_type",
      headerName: "Password encryption",
      width: 150,
      align: "center",
      headerAlign: "center",
      hide: true
    },
    {
      field: "md_sslmode",
      headerName: "SSL Mode",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "md_root_ca_path",
      headerName: "Root CA",
      width: 150,
      align: "center",
      headerAlign: "center",
      hide: true
    },
    {
      field: "md_client_cert_path",
      headerName: "Client cert",
      width: 150,
      align: "center",
      headerAlign: "center",
      hide: true
    },
    {
      field: "md_client_key_path",
      headerName: "Client key",
      width: 150,
      align: "center",
      headerAlign: "center",
      hide: true
    },
    {
      field: "md_group",
      headerName: "Group",
      width: 150,
      align: "center",
      headerAlign: "center",
      hide: true
    },
    {
      field: "md_preset_config_name",
      headerName: "Preset metrics config",
      width: 170,
      align: "center",
      headerAlign: "center",
      hide: true
    },
    {
      field: "md_config",
      headerName: "Custom metrics config",
      width: 170,
      align: "center",
      headerAlign: "center",
      valueGetter: (params) => JSON.stringify(params.value),
      hide: true
    },
    {
      field: "md_preset_config_name_standby",
      headerName: "Standby preset config",
      width: 170,
      align: "center",
      headerAlign: "center",
      hide: true
    },
    {
      field: "md_config_standby",
      headerName: "Standby custom config",
      width: 170,
      align: "center",
      headerAlign: "center",
      valueGetter: (params) => JSON.stringify(params.value),
      hide: true
    },
    {
      field: "md_host_config",
      headerName: "Host config",
      width: 150,
      align: "center",
      headerAlign: "center",
      valueGetter: (params) => JSON.stringify(params.value)
    },
    {
      field: "md_custom_tags",
      headerName: "Custom tags",
      width: 150,
      align: "center",
      headerAlign: "center",
      valueGetter: (params) => JSON.stringify(params.value),
      hide: true
    },
    {
      field: "md_statement_timeout_seconds",
      headerName: "Statement timeout [seconds]",
      type: "number",
      width: 120,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "md_only_if_master",
      headerName: "Master mode only?",
      type: "boolean",
      width: 120,
      renderCell: (params: GridRenderCellParams<boolean>) => <Checkbox checked={params.value} disableRipple />,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "md_last_modified_on",
      headerName: "Last modified",
      type: "string",
      width: 170,
      renderCell: (params) => {
        return (
          <Box sx={{ display: "flex", alignItems: "center", textAlign: "center" }}>
            {moment(params.value).format("DD.MM.YYYY HH:mm:ss")}
          </Box>
        );
      },
      align: "center",
      headerAlign: "center",
    },
    {
      field: "md_is_enabled",
      headerName: "Enabled?",
      type: "boolean",
      width: 120,
      renderCell: (params: GridRenderCellParams<boolean>) => <Checkbox checked={params.value} disableRipple />,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "md_actions",
      headerName: "Actions",
      type: "actions",
      width: 200,
      renderCell: (params: GridRenderCellParams<string>) => (
        <GridActionsComponent
          data={params.row}
          setEditData={setEditData}
          handleModalOpen={handleModalOpen}
          deleteRecord={deleteRecord}
          deleteParameter={params.row.md_unique_name}
          warningMessage={`Remove DB "${params.row.md_unique_name}" from monitoring? NB! This does not remove gathered metrics data from InfluxDB, see bottom of page for that`}
        />
      )
    }
  ];
};
