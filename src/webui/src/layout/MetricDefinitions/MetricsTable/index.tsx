import CheckIcon from '@mui/icons-material/Check';
import CloseIcon from '@mui/icons-material/Close';
import { Box, Typography } from "@mui/material";
import { DataGrid, GridColDef, GridRenderCellParams } from "@mui/x-data-grid";
import moment from 'moment';
import { ActionsComponent } from './ActionsComponent';
import { GridToolbarComponent } from "./GridToolbarComponent";

export const MetricsTable = () => {

  const columns: GridColDef[] = [
    {
      field: "m_name",
      headerName: "Metric name",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "m_versions",
      headerName: "PG versions",
      width: 150,
      align: "center",
      headerAlign: "center",
      valueGetter(params) {
        return (Object.keys(params.value));
      },
    },
    {
      field: "m_comment",
      headerName: "Comment",
      width: 150,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "m_is_active",
      headerName: "Enabled?",
      width: 120,
      renderCell: (params: GridRenderCellParams<boolean>) => params.value ? <CheckIcon /> : <CloseIcon />,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "m_is_helper",
      headerName: "Helpers?",
      width: 120,
      renderCell: (params: GridRenderCellParams<boolean>) => params.value ? <CheckIcon /> : <CloseIcon />,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "m_last_modified_on",
      headerName: "Last modified",
      width: 150,
      renderCell: (params) =>
        <Box sx={{ display: "flex", alignItems: "center", textAlign: "center" }}>
          {moment(params.value).format("MMMM Do YYYY HH:mm:ss")}
        </Box>,
      align: "center",
      headerAlign: "center"
    },
    {
      field: "actions",
      headerName: "Actions",
      type: "actions",
      width: 350,
      renderCell: () => (
        <ActionsComponent />
      ),
      headerAlign: "center"
    }
  ];

  return (
    <Box display="flex" flexDirection="column" gap={1}>
      <Typography variant="h5">
        Metrics
      </Typography>
      <DataGrid
        getRowHeight={() => "auto"}
        getRowId={(row) => row.m_name}
        columns={columns}
        rows={[]}
        autoHeight
        pageSize={5}
        components={{ Toolbar: () => GridToolbarComponent() }}
        disableColumnMenu
      />
    </Box>
  );
};
