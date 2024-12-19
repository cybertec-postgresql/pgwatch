import { makeStyles } from "tss-react/mui";

export const useAlertStyles = makeStyles()(
  () => ({
    root: {
      minWidth: 400,
      maxWidth: 400,
      whiteSpace: "nowrap",
      alignItems: "center",
    },
  }),
);
