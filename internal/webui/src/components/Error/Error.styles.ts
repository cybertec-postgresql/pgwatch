import { makeStyles } from "tss-react/mui";

export const useErrorStyles = makeStyles()(
  () => ({
    root: {
      width: "100%",
      height: "100%",
      display: "flex",
      flexDirection: "column",
      justifyContent: "center",
      alignItems: "center",
      gap: 3,
    },
  })
);
