import { useState } from "react";
import { Alert, AlertColor, Box, Checkbox, Snackbar, Typography } from "@mui/material";
import { DataGrid, GridColDef, GridRenderCellParams } from "@mui/x-data-grid";
import { useQuery } from '@tanstack/react-query';
import moment from 'moment';
import { ErrorComponent } from "layout/common/ErrorComponent";
import { GridToolbarComponent } from "layout/common/GridToolbarComponent";
import { LoadingComponent } from "layout/common/LoadingComponent";
import { QueryKeys } from 'queries/queryKeys';
import { Metric } from 'queries/types/MetricTypes';
import MetricService from 'services/Metric';
import { ActionsComponent } from './ActionsComponent';
import { ModalComponent } from "./ModalComponent";

export const MetricsTable = () => {
  const services = MetricService.getInstance();
  const [alertOpen, setAlertOpen] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [severity, setSeverity] = useState<AlertColor>();
  const [alertText, setAlertText] = useState("");
  const [editData, setEditData] = useState<Metric>();

  const handleAlertOpen = (isOpen: boolean, text: string, type: AlertColor) => {
    setSeverity(type);
    setAlertText(text);
    setAlertOpen(isOpen);
  };

  const handleAlertClose = () => {
    setAlertOpen(false);
  };

  const handleModalOpen = () => {
    setModalOpen(true);
  };

  const handleModalClose = () => {
    setModalOpen(false);
  };

  const { status, data, error } = useQuery<Metric[]>({
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
        <ActionsComponent data={params.row} handleAlertOpen={handleAlertOpen} setEditData={setEditData} handleModalOpen={handleModalOpen} />
      ),
      headerAlign: "center"
    }
  ];

  if (status === "loading") {
    return (
      <LoadingComponent />
    );
  };

  if (status === "error") {
    return (
      <ErrorComponent errorMessage={String(error)} />
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
        getRowId={(row) => row.m_id}
        columns={columns}
        rows={data}
        rowsPerPageOptions={[]}
        components={{ Toolbar: () => <GridToolbarComponent handleModalOpen={handleModalOpen} setEditData={setEditData} /> }}
        disableColumnMenu
      />
      <ModalComponent recordData={editData} open={modalOpen} handleClose={handleModalClose} handleAlertOpen={handleAlertOpen} />
    </Box>
  );
};
