import { Typography } from "@mui/material";
import { useErrorStyles } from "./Error.styles";

type Props = {
  message: string
}

export const Error = ({ message }: Props) => {
  const { classes } = useErrorStyles();

  return (
    <div className={classes.root}>
      <Typography variant="h3" fontWeight="bold">Oops!</Typography>
      <Typography variant="h5">Something went wrong...</Typography>
      <Typography>{message}</Typography>
    </div>
  );
};
