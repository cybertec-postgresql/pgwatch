import { useState } from "react";

import CheckIcon from '@mui/icons-material/Check';
import CloseIcon from '@mui/icons-material/Close';
import { Alert, Box, CircularProgress, Snackbar, Typography } from "@mui/material";
import {
  DataGrid,
  GridColDef,
  GridRenderCellParams
} from "@mui/x-data-grid";

import { useQuery } from "@tanstack/react-query";
import { QueryKeys } from "queries/queryKeys";
import { DB } from "queries/types";
import DbService from "services/Db";

import { ActionsComponent } from "./ActionsComponent";
import { GridToolbarComponent } from "./GridToolbarComponent";
import { ModalComponent } from "./ModalComponent";

export const DbsTable = () => {
  const services = DbService.getInstance();
  const [alertOpen, setAlertOpen] = useState(false);
  const [alertText, setAlertText] = useState("");
  const [modalOpen, setModalOpen] = useState(false);
  const [editData, setEditData] = useState<Record<string, unknown>[]>([]);

  const { status, data } = useQuery<DB[]>({
    queryKey: QueryKeys.db,
    queryFn: async () => {
      return await services.getMonitoredDb();
    }
  });

  const handleAlertOpen = (isOpen: boolean, text: string) => {
    setAlertText(text);

    setAlertOpen(isOpen);
  };

  const handleAlertClose = (event?: React.SyntheticEvent | Event, reason?: string) => {
    if (reason === "clickaway") {
      return;
    }

    setAlertOpen(false);
  };

  const columns: GridColDef[] = [
    {
      field: "DBUniqueName",
      headerName: "Unique name",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "DBType",
      headerName: "DB type",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "Connection",
      headerName: "Connection",
      width: 150,
      align: "center",
      headerAlign: "center",
      valueGetter(params) {
        return (`${params.row.Host}:${params.row.Port}`);
      },
    },
    {
      field: "DBName",
      headerName: "DB dbname",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "DBNameIncludePattern",
      headerName: "DB name inclusion pattern",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "DBNameExcludePattern",
      headerName: "DB name exclusion pattern",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "User",
      headerName: "DB user",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "PasswordType",
      headerName: "Password encryption",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "Helpers",
      headerName: "Auto-create helpers?",
      type: "boolean",
      width: 120,
      renderCell: (params: GridRenderCellParams<boolean>) => {
        if (params.value) {
          return <CheckIcon />;
        } else {
          return <CloseIcon />;
        }
      },
      align: "center",
      headerAlign: "center"
    },
    {
      field: "SslMode",
      headerName: "SSL Mode",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "SslRootCAPath",
      headerName: "Root CA",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "SslClientCertPath",
      headerName: "Client cert",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "SslClientKeyPath",
      headerName: "Client key",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "Group",
      headerName: "Group",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "PresetMetrics",
      headerName: "Preset metrics config",
      width: 170,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "Metrics",
      headerName: "Custom metrics config",
      width: 170,
      align: "center",
      headerAlign: "center",
      valueGetter: (params) => JSON.stringify(params.row.Metrics)
    },
    {
      field: "PresetMetricsStandby",
      headerName: "Standby preset config",
      width: 170,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "MetricsStandby",
      headerName: "Standby custom config",
      width: 170,
      align: "center",
      headerAlign: "center",
      valueGetter: (params) => JSON.stringify(params.row.MetricsStandby)
    },
    {
      field: "HostConfig",
      headerName: "Host config",
      width: 150,
      align: "center",
      headerAlign: "center",
      valueGetter: (params) => JSON.stringify(params.row.HostConfig)
    },
    {
      field: "CustomTags",
      headerName: "Custom tags",
      width: 150,
      align: "center",
      headerAlign: "center",
      valueGetter: (params) => JSON.stringify(params.row.CustomTags)
    },
    {
      field: "StmtTimeout",
      headerName: "Statement timeout [seconds]",
      type: "number",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "OnlyIfMaster",
      headerName: "Master mode only?",
      type: "boolean",
      width: 120,
      renderCell: (params: GridRenderCellParams<boolean>) => {
        if (params.value) {
          return <CheckIcon />;
        } else {
          return <CloseIcon />;
        }
      },
      align: "center",
      headerAlign: "center"
    },
    {
      field: "md_last_modified_on",
      headerName: "Last modified",
      type: "dateTime",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "IsEnabled",
      headerName: "Enabled?",
      type: "boolean",
      width: 120,
      renderCell: (params: GridRenderCellParams<boolean>) => {
        if (params.value) {
          return <CheckIcon />;
        } else {
          return <CloseIcon />;
        }
      },
      align: "center",
      headerAlign: "center"
    },
    {
      field: "actions",
      headerName: "Actions",
      type: "actions",
      width: 200,
      renderCell: (params: GridRenderCellParams<string>) => (
        <ActionsComponent setModalOpen={setModalOpen} data={params.row} setEditData={setEditData} handleAlertOpen={handleAlertOpen} />
      )
    }
  ];

  if (status === "loading") {
    return (
      <Box sx={{ width: "100%", height: 500, display: "flex", justifyContent: "center", alignItems: "center" }}>
        <CircularProgress />
      </Box>
    );
  };

  if (status === "error") {
    return(
      <Box>
        <Typography>Some error happens</Typography>
      </Box>
    );
  };

  return (
    <Box display="flex" flexDirection="column" gap={1}>
      <Snackbar open={alertOpen} autoHideDuration={5000} onClose={handleAlertClose}>
        <Alert sx={{ width: 500 }} variant="filled" severity="success">{alertText}</Alert>
      </Snackbar>
      <Typography variant="h5">
        Databases under monitoring
      </Typography>
      <DataGrid
        columns={columns}
        rows={data}
        getRowId={(row) => row.DBUniqueName}
        autoHeight
        pageSize={5}
        initialState={{
          columns: {
            columnVisibilityModel: {
              DBType: false,
              DBName: false,
              Group: false,
              PresetMetrics: false,
              PresetMetricsStandby: false,
              CustomTags: false,
              IsEnabled: false
            },
          },
        }}
        components={{ Toolbar: () => GridToolbarComponent(setModalOpen, setEditData) }}
        disableColumnMenu
      />
      <ModalComponent open={modalOpen} setOpen={setModalOpen} handleAlertOpen={handleAlertOpen} data={editData} />
    </Box>
  );
};
