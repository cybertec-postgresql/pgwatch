import { usePageStyles } from "styles/page";
import { SourcesGrid } from "./components/SourcesGrid/SourcesGrid";

export const SourcesPage = () => {
  const { classes } = usePageStyles();

  return(
    <div className={classes.root}>
      <SourcesGrid />
    </div>
  );
};
