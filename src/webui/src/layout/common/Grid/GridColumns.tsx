import { useState } from 'react';

import CloseIcon from "@mui/icons-material/Close";
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import OpenInFullIcon from '@mui/icons-material/OpenInFull';
import { Box, Button, Checkbox, Dialog, DialogActions, DialogContent, IconButton, Tooltip, Typography } from "@mui/material";
import { DataGrid, GridColDef, GridRenderCellParams } from "@mui/x-data-grid";

import { UseMutationResult } from "@tanstack/react-query";

import moment from "moment";

import { Db } from "queries/types/DbTypes";
import { Metric } from "queries/types/MetricTypes";
import { Preset, PresetConfigRows } from "queries/types/PresetTypes";

import { DbEnableSwitchComponent } from '../DbEnableSwitchComponent';
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
      field: "DBUniqueName",
      headerName: "Unique name",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "Kind",
      headerName: "DB type",
      width: 150,
      align: "center",
      headerAlign: "center",
      hide: true
    },
    {
      field: "ConnStr",
      headerName: "Connection",
      width: 150,
      align: "center",
      headerAlign: "center",
    },
    {
      field: "IncludePattern",
      headerName: "DB name inclusion pattern",
      width: 150,
      align: "center",
      headerAlign: "center",
      hide: true
    },
    {
      field: "ExcludePattern",
      headerName: "DB name exclusion pattern",
      width: 150,
      align: "center",
      headerAlign: "center",
      hide: true
    },
    {
      field: "IsSuperuser",
      headerName: "Super user?",
      width: 120,
      type: "boolean",
      renderCell: (params: GridRenderCellParams<boolean>) => <Checkbox checked={params.value} disableRipple />,
      align: "center",
      headerAlign: "center",
      hide: true
    },
    {
      field: "Encryption",
      headerName: "Encryption",
      width: 150,
      align: "center",
      headerAlign: "center",
      hide: true
    },
    {
      field: "Group",
      headerName: "Group",
      width: 150,
      align: "center",
      headerAlign: "center",
      hide: true
    },
    {
      field: "PresetMetrics",
      headerName: "Preset metrics config",
      width: 170,
      align: "center",
      headerAlign: "center",
      hide: true
    },
    {
      field: "Metrics",
      headerName: "Custom metrics config",
      width: 170,
      align: "center",
      headerAlign: "center",
      valueGetter: (params) => JSON.stringify(params.value),
      hide: true
    },
    {
      field: "PresetMetricsStandby",
      headerName: "Standby preset config",
      width: 170,
      align: "center",
      headerAlign: "center",
      hide: true
    },
    {
      field: "MetricsStandby",
      headerName: "Standby custom config",
      width: 170,
      align: "center",
      headerAlign: "center",
      valueGetter: (params) => JSON.stringify(params.value),
      hide: true
    },
    {
      field: "HostConfig",
      headerName: "Host config",
      width: 150,
      align: "center",
      headerAlign: "center",
      valueGetter: (params) => JSON.stringify(params.value)
    },
    {
      field: "CustomTags",
      headerName: "Custom tags",
      width: 150,
      align: "center",
      headerAlign: "center",
      valueGetter: (params) => JSON.stringify(params.value),
      hide: true
    },
    {
      field: "OnlyIfMaster",
      headerName: "Master mode only?",
      type: "boolean",
      width: 120,
      renderCell: (params: GridRenderCellParams<boolean>) => <Checkbox checked={params.value} disableRipple />,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "IsEnabled",
      headerName: "Enabled?",
      type: "boolean",
      width: 120,
      renderCell: (params: GridRenderCellParams<boolean>) => <DbEnableSwitchComponent id={params.row.DBUniqueName} value={params.value!} />,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "Actions",
      headerName: "Actions",
      type: "actions",
      width: 330,
      renderCell: (params: GridRenderCellParams<string>) => (
        <GridActionsComponent
          data={params.row}
          setEditData={setEditData}
          handleModalOpen={handleModalOpen}
          deleteRecord={deleteRecord}
          deleteParameter={params.row.DBUniqueName}
          warningMessage={`Remove DB "${params.row.DBUniqueName}" from monitoring? This does not remove gathered metrics data from InfluxDB, see bottom of page for that`}
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
        headerName: "Name",
        width: 150,
        align: "center",
        headerAlign: "center"
      },
      {
        field: "pc_description",
        headerName: "Description",
        width: 400,
        align: "center",
        headerAlign: "center"
      },
      {
        field: "pc_config",
        headerName: "Metrics",
        width: 120,
        align: "center",
        headerAlign: "center",
        renderCell: (params) => {
          const configRows: PresetConfigRows[] = [];

          Object.entries(params.value).map(([key, value]) => configRows.push({ id: key, metric: key, interval: Number(value) }));

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
        field: "metric",
        headerName: "Metric",
        flex: 1,
        align: "center",
        headerAlign: "center"
      },
      {
        field: "interval",
        headerName: "Update interval",
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
      <Typography>{configRows.length}</Typography>
      <Tooltip title="Details">
        <IconButton
          onClick={() => setDialogOpen(true)}
        >
          <OpenInFullIcon />
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
          <Button fullWidth onClick={() => setDialogOpen(false)} variant="contained" startIcon={<CloseIcon />}>Close</Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};
