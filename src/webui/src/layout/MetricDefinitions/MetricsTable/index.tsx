import { useState } from "react";

import { Alert, AlertColor, Box, Snackbar, Typography } from "@mui/material";
import { DataGrid } from "@mui/x-data-grid";

import { useMutation, useQuery } from '@tanstack/react-query';
import { queryClient } from "queryClient";

import { ErrorComponent } from "layout/common/ErrorComponent";
import { metricsColumns } from "layout/common/Grid/GridColumns";
import { GridToolbarComponent } from "layout/common/Grid/GridToolbarComponent";
import { LoadingComponent } from "layout/common/LoadingComponent";

import { QueryKeys } from 'queries/queryKeys';
import { Metric } from 'queries/types/MetricTypes';
import MetricService from 'services/Metric';

import { ModalComponent } from "./ModalComponent";

export const MetricsTable = () => {
  const services = MetricService.getInstance();
  const [alertOpen, setAlertOpen] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [severity, setSeverity] = useState<AlertColor>();
  const [alertText, setAlertText] = useState("");
  const [editData, setEditData] = useState<Metric>();

  const { status, data, error } = useQuery<Metric[]>({
    queryKey: QueryKeys.metric,
    queryFn: async () => {
      return await services.getMetrics();
    }
  });

  const deleteRecord = useMutation({
    mutationFn: async (m_id: number) => {
      return await services.deleteMetric(m_id);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: QueryKeys.metric });
      handleAlertOpen("Metric has been deleted successfully!", "success");
    },
    onError: (error: any) => {
      handleAlertOpen(error.response.data, "error");
    }
  });

  const handleAlertOpen = (text: string, type: AlertColor) => {
    setSeverity(type);
    setAlertText(text);
    setAlertOpen(true);
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

  const columns = metricsColumns(
    {
      setEditData,
      handleModalOpen,
      deleteRecord
    }
  );

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
