import { Alert, Snackbar } from "@mui/material";
import { useAlert } from "utils/AlertContext";

export const AlertComponent = () => {
  const { open, severity, message, closeAlert } = useAlert();

  return (
    <Snackbar open={open} autoHideDuration={3000} onClose={closeAlert}>
      <Alert sx={{ minWidth: 400 }} variant="filled" severity={severity}>{message}</Alert>
    </Snackbar>
  );
};
