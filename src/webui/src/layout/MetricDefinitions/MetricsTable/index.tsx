import CheckIcon from '@mui/icons-material/Check';
import CloseIcon from '@mui/icons-material/Close';
import { Box, Typography } from "@mui/material";
import { DataGrid, GridColDef, GridRenderCellParams } from "@mui/x-data-grid";
import moment from 'moment';
import { ActionsComponent } from './ActionsComponent';
import { GridToolbarComponent } from "./GridToolbarComponent";

export const MetricsTable = () => {

  const data = [
    {
      "m_id": 1,
      "m_name": "wsl",
      "m_version": 14,
      "m_comment": "some comment about metric",
      "m_is_active": true,
      "m_is_helper": false,
      "m_last_modified_on": new Date()
    },
    {
      "m_id": 2,
      "m_name": "backends",
      "m_version": 10.0,
      "m_comment": "some comment about metric",
      "m_is_active": true,
      "m_is_helper": false,
      "m_last_modified_on": new Date()
    }
  ];

  const columns: GridColDef[] = [
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
      field: "m_version",
      headerName: "PG version",
      width: 150,
      type: "number",
      align: "center",
      headerAlign: "center"
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
      width: 200,
      renderCell: (params) => (
        <ActionsComponent data={params.row} />
      ),
      headerAlign: "center"
    }
  ];

  return (
    <Box display="flex" flexDirection="column" gap={1} height="100%">
      <Typography variant="h5">
        Metrics
      </Typography>
      <DataGrid
        getRowHeight={() => "auto"}
        getRowId={(row) => row.m_id}
        columns={columns}
        rows={data}
        rowsPerPageOptions={[]}
        components={{ Toolbar: () => GridToolbarComponent() }}
        disableColumnMenu
      />
    </Box>
  );
};
