import { useState } from 'react';

import CloseIcon from "@mui/icons-material/Close";
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import VisibilityIcon from '@mui/icons-material/Visibility';
import { Box, Button, Checkbox, Dialog, DialogActions, DialogContent, IconButton, Tooltip, Typography } from "@mui/material";
import { DataGrid, GridColDef, GridRenderCellParams } from "@mui/x-data-grid";

import { UseMutationResult } from "@tanstack/react-query";

import moment from "moment";

import { Db } from "queries/types/DbTypes";
import { Metric } from "queries/types/MetricTypes";
import { Preset, PresetConfigRows } from "queries/types/PresetTypes";

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
  handleModalOpen: (state: "NEW" | "EDIT" | "DUPLICATE") => void,
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
      width: 330,
      renderCell: (params: GridRenderCellParams<string>) => (
        <GridActionsComponent
          data={params.row}
          setEditData={setEditData}
          handleModalOpen={handleModalOpen}
          deleteRecord={deleteRecord}
          deleteParameter={params.row.md_unique_name}
          warningMessage={`Remove DB "${params.row.md_unique_name}" from monitoring? NB! This does not remove gathered metrics data from InfluxDB, see bottom of page for that`}
        >
          <Button
            size="small"
            variant="outlined"
            startIcon={<ContentCopyIcon />}
            onClick={() => {
              setEditData(params.row);
              handleModalOpen("DUPLICATE");
            }}
          >
            Duplicate
          </Button>
        </GridActionsComponent>
      )
    }
  ];
};

type presetsColumnsProps = {
  setEditData: React.Dispatch<React.SetStateAction<Preset | undefined>>,
  handleModalOpen: () => void,
  deleteRecord: UseMutationResult<any, any, any, unknown>
};

export const presetsColumns = ({
  setEditData,
  handleModalOpen,
  deleteRecord
}: presetsColumnsProps): GridColDef[] => {
  return (
    [
      {
        field: "pc_name",
        headerName: "Config name",
        width: 150,
        align: "center",
        headerAlign: "center"
      },
      {
        field: "pc_description",
        headerName: "Config description",
        width: 300,
        align: "center",
        headerAlign: "center"
      },
      {
        field: "pc_config",
        headerName: "Config",
        width: 220,
        align: "center",
        headerAlign: "center",
        renderCell: (params) => {
          const configRows: PresetConfigRows[] = [];

          Object.entries(params.value).map(([key, value]) => configRows.push({ id: key, key: key.toUpperCase(), value: Number(value) }));

          return PcConfig(configRows);
        }
      },
      {
        field: "pc_last_modified_on",
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
        field: "actions",
        headerName: "Actions",
        type: "actions",
        width: 200,
        renderCell: (params) => (
          <GridActionsComponent
            data={params.row}
            setEditData={setEditData}
            handleModalOpen={handleModalOpen}
            deleteRecord={deleteRecord}
            deleteParameter={params.row.pc_name}
            warningMessage={`Are you sure you want to delete preset named "${params.row.pc_name}"`}
          />
        ),
        headerAlign: "center"
      }
    ]
  );
};

const pcConfigColumns = (): GridColDef[] => {
  return (
    [
      {
        field: "id",
        headerName: "Id",
        hide: true
      },
      {
        field: "key",
        headerName: "Key",
        flex: 1,
        align: "center",
        headerAlign: "center"
      },
      {
        field: "value",
        headerName: "Value",
        flex: 1,
        align: "center",
        headerAlign: "center"
      }
    ]
  );
};

function PcConfig(configRows: PresetConfigRows[]) {
  const [dialogOpen, setDialogOpen] = useState(false);

  return (
    <Box width="100%" height="100%" display="flex" alignItems="center" justifyContent="space-around">
      <Typography>{`params quantity: ${configRows.length}`}</Typography>
      <Tooltip title="Details">
        <IconButton
          onClick={() => setDialogOpen(true)}
        >
          <VisibilityIcon />
        </IconButton>
      </Tooltip>
      <Dialog open={dialogOpen} onClose={() => setDialogOpen(false)}>
        <DialogContent sx={{ maxHeight: 450, width: 600 }}>
          <DataGrid
            columns={pcConfigColumns()}
            rows={configRows}
            rowsPerPageOptions={[]}
            disableColumnMenu
            autoHeight
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDialogOpen(false)} size="large" variant="contained" endIcon={<CloseIcon />}>Close</Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};
