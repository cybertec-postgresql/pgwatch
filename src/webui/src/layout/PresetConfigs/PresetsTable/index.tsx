import { useState } from "react";

import { Alert, AlertColor, Box, Snackbar, Typography } from "@mui/material";
import { DataGrid } from "@mui/x-data-grid";

import { useMutation, useQuery } from "@tanstack/react-query";
import { queryClient } from "queryClient";

import { ErrorComponent } from "layout/common/ErrorComponent";
import { presetsColumns } from "layout/common/Grid/GridColumns";
import { GridToolbarComponent } from "layout/common/Grid/GridToolbarComponent";
import { LoadingComponent } from "layout/common/LoadingComponent";

import { QueryKeys } from "queries/queryKeys";
import { Preset } from "queries/types/PresetTypes";
import PresetService from "services/Preset";

import { ModalComponent } from "./ModalComponent";


export const PresetsTable = () => {
  const services = PresetService.getInstance();
  const [modalOpen, setModalOpen] = useState(false);
  const [editData, setEditData] = useState<Preset>();
  const [alertOpen, setAlertOpen] = useState(false);
  const [severity, setSeverity] = useState<AlertColor>();
  const [alertText, setAlertText] = useState("");

  const { status, data, error } = useQuery<Preset[]>({
    queryKey: QueryKeys.preset,
    queryFn: async () => {
      return await services.getPresets();
    }
  });

  const deleteRecord = useMutation({
    mutationFn: async (pc_name: string) => {
      return await services.deletePreset(pc_name);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: QueryKeys.preset });
      handleAlertOpen("Preset has been successfully deleted!", "success");
    },
    onError: (resultError: any) => {
      handleAlertOpen(resultError.response.data, "error");
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
      <Snackbar open={alertOpen} autoHideDuration={5000} onClose={handleAlertClose}>
        <Alert sx={{ width: "auto" }} variant="filled" severity={severity}>{alertText}</Alert>
      </Snackbar>
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
