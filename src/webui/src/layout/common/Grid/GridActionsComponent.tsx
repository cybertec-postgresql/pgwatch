import { useState } from 'react';

import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import { Box, Button, Dialog, DialogActions, DialogContent, DialogContentText, DialogTitle } from "@mui/material";

import { UseMutationResult } from "@tanstack/react-query";


type Props = {
  data: any,
  setEditData: React.Dispatch<React.SetStateAction<any>>
  handleModalOpen: () => void,
  deleteRecord: UseMutationResult<any, any, any, unknown>,
  deleteParameter: number | string,
  warningMessage: string
}

export const GridActionsComponent = (
  {
    data,
    setEditData,
    handleModalOpen,
    deleteRecord,
    deleteParameter,
    warningMessage
  }: Props
) => {
  const [dialogOpen, setDialogOpen] = useState(false);

  const handleEditClick = () => {
    setEditData(data);
    handleModalOpen();
  };

  const handleDeleteClick = () => {
    deleteRecord.mutate(deleteParameter);
  };

  const handleDialogOpen = () => {
    setDialogOpen(true);
  };

  const handleDialogClose = () => {
    setDialogOpen(false);
  };

  return (
    <Box sx={{ display: "flex", justifyContent: "space-between", width: "100%" }}>
      <Button
        size="small"
        variant="outlined"
        startIcon={<EditIcon />}
        onClick={handleEditClick}
      >
        Edit
      </Button>
      <Button
        size="small"
        variant="contained"
        startIcon={<DeleteIcon />}
        onClick={handleDialogOpen}
      >
        Delete
      </Button>
      <Dialog open={dialogOpen} onClose={handleDialogClose}>
        <DialogTitle>Warning</DialogTitle>
        <DialogContent>
          <DialogContentText>
            {warningMessage}
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleDeleteClick}>Ok</Button>
          <Button onClick={handleDialogClose}>Cancel</Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};
