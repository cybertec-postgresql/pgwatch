import { makeStyles } from "tss-react/mui";

export const useLogsStyles = makeStyles()(
  () => ({
    root: {
      backgroundColor: "black",
      flexGrow: 1,
      position: "relative",
      width: "100%",
      height: "100%",
    },
    grid: {
      color: "white",
      position: "absolute",
      inset: "1em",
      overflowY: "auto",
      overflowX: "hidden",
      wordWrap: "break-word",
      whiteSpace: "pre-wrap",
      fontSize: "14px",
      margin: 0
    },
    log: {
      margin: "0 0 0.5em",
    },
  }),
);
