import { useEffect, useState } from "react";
import { Loading } from "components/Loading/Loading";
import { ReadyState } from "react-use-websocket";
import { usePageStyles } from "styles/page";
import { useLogs } from "queries/Log";
import { useAlert } from "utils/AlertContext";
import { useLogsStyles } from "./Logs.styles";
import { LogOutput } from "./components/LogOutput";

export const Logs = () => {
  const { classes: pageClasses } = usePageStyles();
  const { classes: logsClasses } = useLogsStyles();

  const [logsHistory, setLogsHistory] = useState<MessageEvent<any>[]>([]);
  const { lastMessage, readyState } = useLogs();
  const { callAlert } = useAlert();

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
    <div className={pageClasses.page}>
      <div className={logsClasses.root}>
        {readyState === ReadyState.CONNECTING && <Loading />}
        <pre className={logsClasses.grid}>
          {[...logsHistory].reverse().map((log, index) => (
            <LogOutput log={log} index={index} key={index} />
          ))}
        </pre>
      </div>
    </div>
  );
};
