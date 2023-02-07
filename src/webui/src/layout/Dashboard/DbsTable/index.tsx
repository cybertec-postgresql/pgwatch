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
import moment from "moment";
import { QueryKeys } from "queries/queryKeys";
import { Db } from "queries/types";
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

  const { status, data } = useQuery<Db[]>({
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
      field: "md_id",
      headerName: "ID",
      width: 75,
      type: "number",
      align: "center",
      headerAlign: "center"
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
      headerAlign: "center"
    },
    {
      field: "Connection",
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
      headerAlign: "center"
    },
    {
      field: "md_include_pattern",
      headerName: "DB name inclusion pattern",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "md_exclude_pattern",
      headerName: "DB name exclusion pattern",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "md_user",
      headerName: "DB user",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "md_password_type",
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
      headerAlign: "center"
    },
    {
      field: "md_client_cert_path",
      headerName: "Client cert",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "md_client_key_path",
      headerName: "Client key",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "md_group",
      headerName: "Group",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "md_preset_config_name",
      headerName: "Preset metrics config",
      width: 170,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "md_config",
      headerName: "Custom metrics config",
      width: 170,
      align: "center",
      headerAlign: "center",
      valueGetter: (params) => JSON.stringify(params.row.Metrics)
    },
    {
      field: "md_preset_config_name_standby",
      headerName: "Standby preset config",
      width: 170,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "md_config_standby",
      headerName: "Standby custom config",
      width: 170,
      align: "center",
      headerAlign: "center",
      valueGetter: (params) => JSON.stringify(params.row.MetricsStandby)
    },
    {
      field: "md_host_config",
      headerName: "Host config",
      width: 150,
      align: "center",
      headerAlign: "center",
      valueGetter: (params) => JSON.stringify(params.row.HostConfig)
    },
    {
      field: "md_custom_tags",
      headerName: "Custom tags",
      width: 150,
      align: "center",
      headerAlign: "center",
      valueGetter: (params) => JSON.stringify(params.row.CustomTags)
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
      type: "string",
      width: 150,
      renderCell: (params) => {
        return (
          <Box sx={{ display: "flex", alignItems: "center", textAlign: "center" }}>
            {moment(params.value).format("MMMM Do YYYY hh:mm:ss")}
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
    return (
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
        getRowHeight={() => "auto"}
        columns={columns}
        rows={data}
        getRowId={(row) => row.md_unique_name}
        autoHeight
        pageSize={5}
        initialState={{
          columns: {
            columnVisibilityModel: {
              md_id: false,
              md_dbtype: false,
              md_dbname: false,
              md_password_type: false,
              md_include_pattern: false,
              md_exclude_pattern: false,
              md_root_ca_path: false,
              md_client_cert_path: false,
              md_client_key_path: false,
              md_group: false,
              md_config: false,
              md_preset_config_name: false,
              md_preset_config_name_standby: false,
              md_config_standby: false,
              md_custom_tags: false,
              md_is_enabled: true
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
