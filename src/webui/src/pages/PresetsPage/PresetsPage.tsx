import { usePageStyles } from "styles/page";
import { PresetsGrid } from "./components/PresetsGrid/PresetsGrid";

export const PresetsPage = () => {
  const { classes } = usePageStyles();

  return (
    <div className={classes.root}>
      <PresetsGrid />
    </div>
  );
};
