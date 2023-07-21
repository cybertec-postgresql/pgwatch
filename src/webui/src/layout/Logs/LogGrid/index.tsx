import { useEffect, useState } from "react";
import { Box } from "@mui/material";
import { ReadyState } from "react-use-websocket/dist/lib/constants";
import { LoadingComponent } from "layout/common/LoadingComponent";
import { useLogs } from "queries/Log";
import { useAlert } from "utils/AlertContext";
import style from "../style.module.css";
import { logOutput } from "./logOutput";

export const LogGrid = () => {
  const { callAlert } = useAlert();
  const [logsHistory, setLogsHistory] = useState<MessageEvent<any>[]>([]);
  const { lastMessage, readyState } = useLogs();

  useEffect(() => {
    if (lastMessage !== null) {
      setLogsHistory((prev) => prev.concat(lastMessage));
    }
  }, [lastMessage]);

  useEffect(() => {
    switch (readyState) {
      case ReadyState.OPEN:
        callAlert("success", "Connection established");
        break;
      case ReadyState.CLOSED:
        callAlert("error", "Connection closed");
        break;
      case ReadyState.UNINSTANTIATED:
        callAlert("error", "Connection uninstantiated");
        break;
    }
  }, [readyState, callAlert]);

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
    </Box>
  );
};
