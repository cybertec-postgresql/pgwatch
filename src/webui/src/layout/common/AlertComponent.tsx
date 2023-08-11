import { Alert, Snackbar, Tooltip, Typography } from "@mui/material";
import { useAlert } from "utils/AlertContext";

export const AlertComponent = () => {
  const { open, severity, message, closeAlert } = useAlert();

  return (
    <Snackbar open={open} autoHideDuration={4000} onClose={closeAlert}>
      <Alert sx={{ minWidth: 400, maxWidth: 400, whiteSpace: "nowrap", alignItems: "center" }} variant="filled" severity={severity}>
        <Tooltip title={message}>
          <Typography noWrap>
            {message}
          </Typography>
        </Tooltip>
      </Alert>
    </Snackbar>
  );
};
