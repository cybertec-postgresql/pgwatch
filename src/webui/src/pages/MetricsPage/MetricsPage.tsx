import { usePageStyles } from "styles/page";
import { MetricsGrid } from "./components/MetricsGrid/MetricsGrid";

export const MetricsPage = () => {

  const { classes } = usePageStyles();

  return (
    <div className={classes.root}>
      <MetricsGrid />
    </div>
  );
};
