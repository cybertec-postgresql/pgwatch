import { Button, Dialog, DialogActions, DialogContent, DialogContentText, DialogTitle } from "@mui/material";

type Props = {
  open: boolean;
  message: string;
  onClose: () => void;
  onSubmit: () => void;
};

export const WarningDialog = (props: Props) => {
  const {onSubmit, onClose, open, message} = props;

  return (
    <Dialog open={open}>
      <DialogTitle>Warning</DialogTitle>
      <DialogContent>
        <DialogContentText>
          {message}
        </DialogContentText>
      </DialogContent>
      <DialogActions>
        <Button onClick={onSubmit}>Ok</Button>
        <Button variant="contained" onClick={onClose}>Cancel</Button>
      </DialogActions>
    </Dialog>
  );
};
