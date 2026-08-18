import { makeStyles } from 'tss-react/mui';

export const useGridActionsStyles = makeStyles()(
  () => ({
    root: {
      display: "flex",
      width: "100%",
      gap: 4,
      justifyContent: "center",
    },
  }),
);
