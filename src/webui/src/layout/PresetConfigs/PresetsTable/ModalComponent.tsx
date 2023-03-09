import CloseIcon from "@mui/icons-material/Close";
import DoneIcon from "@mui/icons-material/Done";
import { Button, Dialog, DialogActions, DialogContent, DialogTitle } from "@mui/material";

import { Preset } from "queries/types/PresetTypes";


type Props = {
  open: boolean,
  handleClose: () => void,
  recordData: Preset | undefined;
};

export const ModalComponent = ({ open, handleClose, recordData }: Props) => {

  return (
    <Dialog
      onClose={handleClose}
      open={open}
      fullWidth
      maxWidth="md"
    >
      <DialogTitle>{recordData ? "Edit monitored database" : "Add new database to monitoring"}</DialogTitle>
      <form>
        <DialogContent>
          Dialog Content
        </DialogContent>
        <DialogActions>
          <Button fullWidth onClick={handleClose} size="medium" variant="outlined" startIcon={<CloseIcon />}>Cancel</Button>
          <Button fullWidth onClick={handleClose} size="medium" variant="contained" startIcon={<DoneIcon />}>{recordData ? "Submit changes" : "Start monitoring"}</Button>
        </DialogActions>
      </form>
    </Dialog>
  );
};
