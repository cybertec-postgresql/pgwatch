import { useMemo } from "react";
import { useLogsStyles } from "../Logs.styles";
import { logEventColor } from "./LogOutput.consts";

type Props = {
  log: MessageEvent<any>;
  index: number;
};

export const LogOutput = ({ log, index }: Props) => {
  const { classes } = useLogsStyles();

  const splittedLog = useMemo(
    () => String(log.data).split(" "),
    [log.data],
  );

  return (
    <p className={classes.log}>
      {
        splittedLog.map((val, idx) => {
          if (logEventColor[val]) {
            return (<span key={index} style={{ color: logEventColor[val] }}> {val}</span>);
          } else {
            if (idx === 0) {
              return val;
            }
            return " " + val;
          }
        })
      }
    </p>
  );
};
