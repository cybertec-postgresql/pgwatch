import { CircularProgress } from "@mui/material";
import { useLoadingStyles } from "./Loading.styles";

export const Loading = () => {
  const { classes } = useLoadingStyles();

  return (
    <div className={classes.root}>
      <CircularProgress size={70} />
    </div>
  );
};
