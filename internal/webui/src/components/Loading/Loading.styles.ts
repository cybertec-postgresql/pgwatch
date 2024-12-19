import { makeStyles } from "tss-react/mui";

export const useLoadingStyles = makeStyles()(
  () => ({
    root: {
      width: "100%",
      height: "100%",
      display: "flex",
      justifyContent: "center",
      alignItems: "center",
    },
  }),
);
