import { useState } from "react";

import CheckIcon from '@mui/icons-material/Check';
import CloseIcon from '@mui/icons-material/Close';
import { Alert, Box, Snackbar, Typography } from "@mui/material";
import {
  DataGrid,
  GridColDef,
  GridRenderCellParams
} from "@mui/x-data-grid";

import { ActionsComponent } from "./ActionsComponent";
import { GridToolbarComponent } from "./GridToolbarComponent";
import { ModalComponent } from "./ModalComponent";
import { PasswordTypography } from "./PasswordTypographyComponent";

const mockRows = [
  {
    id: 1,
    md_unique_name: "test",
    md_dbtype: "patroni",
    md_hostname: "localhost",
    md_port: 5433,
    md_dbname: "diploma",
    md_user: "cybertec-admin",
    md_password: "cybertec_super_duper_puper_admin",
    md_last_modified_on: new Date().toUTCString(),
    md_password_type: "plain-text",
    helpers: true,
    md_sslmode: "verify-ca",
    md_preset_config_name: "exhaustive",
    md_preset_config_name_standby: "standard"
  },
  {
    id: 2,
    md_unique_name: "test2",
    md_dbtype: "pgpool",
    md_hostname: "192.168.0.1",
    md_port: 6214,
    md_dbname: "cybertec_pgwatch2",
    md_user: "qwertyui12345",
    md_password: "simple",
    md_last_modified_on: new Date('2021-08-10T03:24:00').toUTCString(),
    md_password_type: "aes-gcm-256",
    md_sslmode: "verify-full",
    md_preset_config_name: "superuser_no_python",
    md_preset_config_name_standby: "rds"
  }
];

export const DbsTable = () => {
  const [alertOpen, setAlertOpen] = useState(false);
  const [alertText, setAlertText] = useState("");
  const [modalOpen, setModalOpen] = useState(false);
  const [editData, setEditData] = useState<Record<string, unknown>[]>([]);

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
      field: "id",
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
      field: "connection",
      headerName: "Connection",
      width: 150,
      align: "center",
      headerAlign: "center",
      valueGetter(params) {
        return(`${params.row.md_hostname}:${params.row.md_port}`);
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
      field: "md_password",
      headerName: "DB password",
      renderCell: (params: GridRenderCellParams<string>) => (
        <PasswordTypography value={params.value!} />
      ),
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
      field: "helpers",
      headerName: "Auto-create helpers?",
      type: "boolean",
      width: 120,
      renderCell: (params: GridRenderCellParams<boolean>) => {
        if(params.value) {
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
      headerAlign: "center"
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
      headerAlign: "center"
    },
    {
      field: "md_host_config",
      headerName: "Host config",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "md_custom_tags",
      headerName: "Custom tags",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "md_statement_timeout_seconds",
      headerName: "Statement timeout [seconds]",
      type: "number",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "md_only_if_master",
      headerName: "Master mode only?",
      type: "boolean",
      width: 120,
      renderCell: (params: GridRenderCellParams<boolean>) => {
        if(params.value) {
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
      field: "md_is_enabled",
      headerName: "Enabled?",
      type: "boolean",
      width: 120,
      renderCell: (params: GridRenderCellParams<boolean>) => {
        if(params.value) {
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
        <ActionsComponent setModalOpen={setModalOpen} data={params.row} setEditData={setEditData} />
      )
    }
  ];

  const rows = mockRows; // TODO: get this data from the api

  return (
    <Box display="flex" flexDirection="column" gap={1}>
      <Snackbar open={alertOpen} autoHideDuration={3000} onClose={handleAlertClose}>
        <Alert sx={{ width: 500 }} variant="filled" severity="success">{alertText}</Alert>
      </Snackbar>
      <Typography variant="h5">
        Databases under monitoring
      </Typography>
      <DataGrid
        getRowHeight={() => "auto"}
        columns={columns}
        rows={rows}
        autoHeight
        pageSize={5}
        initialState={{
          columns: {
            columnVisibilityModel: {
              id: false,
              md_dbtype: false,
              md_dbname: false,
              md_group: false,
              md_preset_config_name: false,
              md_preset_config_name_standby: false,
              md_custom_tags: false,
              md_is_enabled: false
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
