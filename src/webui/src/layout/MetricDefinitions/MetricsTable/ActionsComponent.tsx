import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import { Box, Button } from "@mui/material";
import { useState } from 'react';
import { ModalComponent } from './ModalComponent';

type Props = {
  data: any
};

export const ActionsComponent = ({ data }: Props) => {
  const [openModal, setOpenModal] = useState(false);

  const handleClose = () => setOpenModal(false);

  const handleOpen = () => setOpenModal(true);

  return (
    <Box sx={{ display: "flex", justifyContent: "space-between", width: "100%" }}>
      <Button
        size="small"
        variant="outlined"
        startIcon={<EditIcon />}
        onClick={handleOpen}
      >
        Edit
      </Button>
      <Button
        size="small"
        variant="contained"
        startIcon={<DeleteIcon />}
      >
        Delete
      </Button>
    </Box>
  );
};
