import { useMemo, useState } from "react";

import { Box, Typography } from "@mui/material";
import { DataGrid } from "@mui/x-data-grid";

import { ErrorComponent } from "layout/common/ErrorComponent";
import { GridToolbarComponent } from "layout/common/Grid/GridToolbarComponent";
import { LoadingComponent } from "layout/common/LoadingComponent";

import { useDeleteMetric, useMetrics } from "queries/Metric";

import { ModalComponent } from "./ModalComponent";
import { metricsColumns } from "../MetricDefinitions.consts";
import { MetricRow } from "../MetricDefinitions.types";

export const MetricsTable = () => {
  const [modalOpen, setModalOpen] = useState(false);
  const [editData, setEditData] = useState();

  const { status, data, error } = useMetrics();

  const rows: MetricRow[] | [] = useMemo(() => {
    if (data) {
      return Object.keys(data).map((key) => {
        const metric = data[key];
        return {
          Key: key,
          Metric: metric,
        };
      });
    }
    return [];
  }, [data]);

  const deleteRecord = useDeleteMetric();

  const handleModalOpen = () => {
    setModalOpen(true);
  };

  const handleModalClose = () => {
    setModalOpen(false);
  };

  const columns = metricsColumns();

  if (status === "loading") {
    return (
      <LoadingComponent />
    );
  };

  if (status === "error") {
    const err = error as Error;
    return (
      <ErrorComponent errorMessage={err.message} />
    );
  };

  return (
    <Box display="flex" flexDirection="column" gap={1} height="100%">
      <Typography variant="h5">
        Metrics
      </Typography>
      <DataGrid
        getRowId={(row) => row.Key}
        columns={columns}
        rows={rows}
        rowsPerPageOptions={[]}
        components={{ Toolbar: () => <GridToolbarComponent handleModalOpen={handleModalOpen} setEditData={setEditData} /> }}
        disableColumnMenu
      />
      <ModalComponent recordData={editData} open={modalOpen} handleClose={handleModalClose} />
    </Box>
  );
};
