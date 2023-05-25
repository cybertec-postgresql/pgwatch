import { useEffect, useState } from "react";
import { Alert, AlertColor, Box, Snackbar } from "@mui/material";
import { ReadyState } from "react-use-websocket/dist/lib/constants";
import { LoadingComponent } from "layout/common/LoadingComponent";
import { useLogs } from "queries/Log";
import style from "../style.module.css";
import { logOutput } from "./logOutput";

export const LogGrid = () => {
  const [alertOpen, setAlertOpen] = useState(false);
  const [alertSeverity, setAlertSeverity] = useState<AlertColor>();
  const [alertText, setAlertText] = useState("");
  const [logsHistory, setLogsHistory] = useState<MessageEvent<any>[]>([]);
  const { lastMessage, readyState } = useLogs();

  useEffect(() => {
    if (lastMessage !== null) {
      setLogsHistory((prev) => prev.concat(lastMessage));
    }
  }, [lastMessage]);

  const handleAlertOpen = (text: string, type: AlertColor) => {
    setAlertSeverity(type);
    setAlertText(text);
    setAlertOpen(true);
  };

  const handleAlertClose = () => {
    setAlertOpen(false);
  };

  useEffect(() => {
    switch (readyState) {
      case ReadyState.OPEN:
        handleAlertOpen("Connection successfully established", "success");
        break;
      case ReadyState.CLOSED:
        handleAlertOpen("Connection is closed", "error");
        break;
      case ReadyState.UNINSTANTIATED:
        handleAlertOpen("Connection isn't instantiated", "error");
        break;
    }
  }, [readyState]);

  return (
    <Box display="flex" flexDirection="column" height="100%">
      <Box
        sx={{
          bgcolor: "black",
          flexGrow: 1,
          position: "relative",
          width: "100%",
          height: "100%"
        }}
      >
        {
          readyState === ReadyState.CONNECTING && (
            <LoadingComponent />
          )
        }
        <pre className={style.LogGrid}>
          {
            [...logsHistory].reverse().map(logOutput)
          }
        </pre>
      </Box>
      <Snackbar open={alertOpen} autoHideDuration={5000} onClose={handleAlertClose}>
        <Alert sx={{ width: "auto" }} variant="filled" severity={alertSeverity}>{alertText}</Alert>
      </Snackbar>
    </Box>
  );
};
