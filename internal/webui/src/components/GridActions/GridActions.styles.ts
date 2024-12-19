import { makeStyles } from 'tss-react/mui';

export const useGridActionsStyles = makeStyles()(
  () => ({
    root: {
      display: "flex",
      width: "100%",
      justifyContent: "space-between",
    },
  }),
);
