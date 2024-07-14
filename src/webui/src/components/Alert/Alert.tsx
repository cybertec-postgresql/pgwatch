import { Alert as MuiAlert, Snackbar, Tooltip, Typography } from "@mui/material";
import { useAlert } from "utils/AlertContext";
import { useAlertStyles } from "./Alert.styles";

export const Alert = () => {
  const { open, severity, message, closeAlert } = useAlert();

  const { classes } = useAlertStyles();

  return (
    <Snackbar open={open} autoHideDuration={4000} onClose={closeAlert}>
      <MuiAlert
        className={classes.root}
        variant="filled"
        severity={severity}
      >
        <Tooltip title={message}>
          <Typography noWrap>
            {message}
          </Typography>
        </Tooltip>
      </MuiAlert>
    </Snackbar>
  );
};
