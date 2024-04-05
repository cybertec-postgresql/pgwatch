import { useMetricsPageStyles } from "./MetricsPage.styles";
import { MetricsGrid } from "./components/MetricsGrid/MetricsGrid";

export const MetricsPage = () => {
  const classes = useMetricsPageStyles();

  return(
    <div className={classes.root}>
      <MetricsGrid />
    </div>
  );
};
