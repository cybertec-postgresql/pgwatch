import { makeStyles } from "tss-react/mui";

export const usePageStyles = makeStyles()(
  () => ({
    root: {
      flex: "1 1 auto",
    },
    page: {
      display: "flex",
      flexDirection: "column",
      gap: 1,
      height: "100%",
    },
  }),
);
