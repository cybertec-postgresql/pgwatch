import { makeStyles } from "tss-react/mui";

export const useLoginFormStyles = makeStyles()(
  () => ({
    form: {
      width: "270px",
      display: "flex",
      flexDirection: "column",
      gap: "24px",
    },
  })
);
