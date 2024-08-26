import { usePageStyles } from "styles/page";
import { Logs } from "./components/Logs";

export const LogsPage = () => {
  const { classes } = usePageStyles();

  return (
    <div className={classes.root}>
      <Logs />
    </div>
  );
};
