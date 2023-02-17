import { useState } from "react";
import { Alert, AlertColor, Box, Checkbox, CircularProgress, Snackbar, Typography } from "@mui/material";
import { DataGrid, GridColDef, GridRenderCellParams } from "@mui/x-data-grid";
import { useQuery } from '@tanstack/react-query';
import moment from 'moment';
import { QueryKeys } from 'queries/queryKeys';
import { Metric } from 'queries/types/MetricTypes';
import MetricService from 'services/Metric';
import { ActionsComponent } from './ActionsComponent';
import { GridToolbarComponent } from "./GridToolbarComponent";

export const MetricsTable = () => {
  const services = MetricService.getInstance();
  const [alertOpen, setAlertOpen] = useState(false);
  const [severity, setSeverity] = useState<AlertColor>();
  const [alertText, setAlertText] = useState("");

  const handleAlertOpen = (isOpen: boolean, text: string, type: AlertColor) => {
    setSeverity(type);
    setAlertText(text);
    setAlertOpen(isOpen);
  };

  const handleAlertClose = () => {
    setAlertOpen(false);
  };

  const { status, data } = useQuery<Metric[]>({
    queryKey: QueryKeys.metric,
    queryFn: async () => {
      return await services.getMetrics();
    }
  });

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
        <ActionsComponent data={params.row} handleAlertOpen={handleAlertOpen} />
      ),
      headerAlign: "center"
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
    <Box display="flex" flexDirection="column" gap={1} height="100%">
      <Snackbar open={alertOpen} autoHideDuration={5000} onClose={handleAlertClose}>
        <Alert sx={{ width: "auto" }} variant="filled" severity={severity}>{alertText}</Alert>
      </Snackbar>
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
