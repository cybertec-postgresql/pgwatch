import { useState } from "react";

import { Box, Typography } from "@mui/material";
import { DataGrid } from "@mui/x-data-grid";

import { useNavigate } from "react-router-dom";

import { ErrorComponent } from "layout/common/ErrorComponent";
import { presetsColumns } from "layout/common/Grid/GridColumns";
import { GridToolbarComponent } from "layout/common/Grid/GridToolbarComponent";
import { LoadingComponent } from "layout/common/LoadingComponent";

import { useDeletePreset, usePresets } from "queries/Preset";
import { Preset } from "queries/types/PresetTypes";

import { useAlert } from "utils/AlertContext";
import { ModalComponent } from "./ModalComponent";


export const PresetsTable = () => {
  const { callAlert } = useAlert();
  const navigate = useNavigate();
  const [modalOpen, setModalOpen] = useState(false);
  const [editData, setEditData] = useState<Preset>();

  const { status, data, error } = usePresets(callAlert, navigate);

  const deleteRecord = useDeletePreset(callAlert, navigate);

  const handleModalOpen = () => {
    setModalOpen(true);
  };

  const handleModalClose = () => {
    setModalOpen(false);
  };

  const columns = presetsColumns(
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
        Preset configs
      </Typography>
      <DataGrid
        getRowId={(row) => row.pc_name}
        columns={columns}
        rows={data}
        rowsPerPageOptions={[]}
        components={{ Toolbar: () => <GridToolbarComponent handleModalOpen={handleModalOpen} setEditData={setEditData} /> }}
        disableColumnMenu
      />
      <ModalComponent open={modalOpen} handleClose={handleModalClose} recordData={editData} />
    </Box>
  );
};
