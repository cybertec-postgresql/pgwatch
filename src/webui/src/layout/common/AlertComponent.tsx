import { useState } from "react";
import { Alert, AlertColor, Snackbar } from "@mui/material";

type Props = {
  severity: AlertColor,
  message: string
};

export const AlertComponent = ({ severity, message }: Props) => {
  const [open, setOpen] = useState(true);

  const handleClose = () => {
    setOpen(false);
  };

  return (
    <Snackbar open={open} autoHideDuration={5000} onClose={handleClose}>
      <Alert sx={{ width: "auto" }} variant="filled" severity={severity}>{message}</Alert>
    </Snackbar>
  );
};
