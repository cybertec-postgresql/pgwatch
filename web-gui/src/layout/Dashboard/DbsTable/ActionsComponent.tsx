import { useState } from "react";
import { Box, Button, Dialog, DialogActions, DialogContent, DialogContentText, DialogTitle, TextField } from "@mui/material";

import EditIcon from '@mui/icons-material/Edit';
import DeleteIcon from '@mui/icons-material/Delete';

type Params = {
  data: any,
  setModalOpen: any,
  setEditData: any
}

export const ActionsComponent = ({ data, setModalOpen, setEditData }: Params) => {
  const [deleteClicked, setDeleteClicked] = useState(false);

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
      <Button onClick={() => {setModalOpen(true); setEditData(data)}} sx={{ marginRight: "7.5px", marginLeft: "2.5px" }}
        size="small" variant="outlined" startIcon={<EditIcon />}>Edit</Button>

      <Button onClick={handleDeleteOpen} size="small" variant="contained" startIcon={<DeleteIcon />}>Delete</Button>
      <Dialog open={deleteClicked} onClose={handleDeleteClose}>
        <DialogTitle>Warning</DialogTitle>
        <DialogContent>
          <DialogContentText>
            {`Remove DB "${data.uniqueName}" from monitoring? NB! This does not remove gathered metrics data from InfluxDB, see bottom of page for that`}
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleDeleteClose}>Ok</Button>
          <Button onClick={handleDeleteClose}>Cancel</Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};
