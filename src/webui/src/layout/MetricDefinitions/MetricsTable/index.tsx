import { useState } from "react";

import { Box, Typography } from "@mui/material";
import { DataGrid } from "@mui/x-data-grid";

import { useNavigate } from "react-router-dom";

import { ErrorComponent } from "layout/common/ErrorComponent";
import { metricsColumns } from "layout/common/Grid/GridColumns";
import { GridToolbarComponent } from "layout/common/Grid/GridToolbarComponent";
import { LoadingComponent } from "layout/common/LoadingComponent";

import { useDeleteMetric, useMetrics } from "queries/Metric";
import { Metric } from 'queries/types/MetricTypes';

import { useAlert } from "utils/AlertContext";

import { ModalComponent } from "./ModalComponent";

export const MetricsTable = () => {
  const { callAlert } = useAlert();
  const navigate = useNavigate();
  const [modalOpen, setModalOpen] = useState(false);
  const [editData, setEditData] = useState<Metric>();

  const { status, data, error } = useMetrics(callAlert, navigate);

  const deleteRecord = useDeleteMetric(callAlert, navigate);

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
      <ModalComponent recordData={editData} open={modalOpen} handleClose={handleModalClose} />
    </Box>
  );
};
