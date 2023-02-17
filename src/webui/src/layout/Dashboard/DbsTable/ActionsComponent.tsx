import { useState } from "react";

import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import { AlertColor, Box, Button, Dialog, DialogActions, DialogContent, DialogContentText, DialogTitle } from "@mui/material";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { QueryKeys } from "queries/queryKeys";
import { Db } from "queries/types/DbTypes";
import DbService from "services/Db";


type Params = {
  data: Db,
  setModalOpen: React.Dispatch<React.SetStateAction<boolean>>,
  setEditData: React.Dispatch<React.SetStateAction<Db | undefined>>,
  handleAlertOpen: (isOpen: boolean, text: string, type: AlertColor) => void
}

export const ActionsComponent = ({ data, setModalOpen, setEditData, handleAlertOpen }: Params) => {
  const services = DbService.getInstance();
  const [deleteClicked, setDeleteClicked] = useState(false);
  const queryClient = useQueryClient();

  const deleteRecord = useMutation({
    mutationFn: async (uniqueName: string) => {
      return await services.deleteMonitoredDb(uniqueName);
    },
    onSuccess: () => {
      setDeleteClicked(false);
      queryClient.invalidateQueries({ queryKey: QueryKeys.db });
      handleAlertOpen(true, `Monitored DB "${data.md_unique_name}" has been deleted successfully!`, "success");
    }
  });

  const handleDeleteOpen = () => {
    setDeleteClicked(true);
  };

  const handleDeleteClose = (event?: {}, reason?: string) => {
    if (reason === "backdropClick") {
      return;
    }

    setDeleteClicked(false);
  };

  return (
    <Box sx={{ display: "flex", justifyContent: "center", alignItems: "center" }}>
      <Button
        onClick={() => { setModalOpen(true); setEditData(data); }}
        sx={{ marginRight: "7.5px", marginLeft: "2.5px" }}
        size="small"
        variant="outlined"
        startIcon={<EditIcon />}
      >
        Edit
      </Button>
      <Button onClick={handleDeleteOpen} size="small" variant="contained" startIcon={<DeleteIcon />}>Delete</Button>
      <Dialog open={deleteClicked} onClose={handleDeleteClose}>
        <DialogTitle>Warning</DialogTitle>
        <DialogContent>
          <DialogContentText>
            {`Remove DB "${data.md_unique_name}" from monitoring? NB! This does not remove gathered metrics data from InfluxDB, see bottom of page for that`}
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => deleteRecord.mutate(data.md_unique_name)}>Ok</Button>
          <Button onClick={handleDeleteClose}>Cancel</Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};
