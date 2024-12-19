import { makeStyles } from "tss-react/mui";

export const useLoginPageStyles = makeStyles()(
  () => ({
    root: {
      display: "flex",
      justifyContent: "center",
      alignItems: "center",
      flexDirection: "column",
      gap: "5px",
    },
  }),
);
