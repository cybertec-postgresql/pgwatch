import AddIcon from "@mui/icons-material/Add";
import { Button } from "@mui/material";
import { GridToolbarColumnsButton, GridToolbarContainer, GridToolbarFilterButton } from "@mui/x-data-grid";
import { useGridToolbarStyles } from "./GridToolbar.styles";

type Props = {
  onNewClick: () => void;
  children?: React.ReactNode;
};

export const GridToolbar = (props: Props) => {
  const { onNewClick, children } = props;

  const classes = useGridToolbarStyles();

  return (
    <GridToolbarContainer>
      <GridToolbarColumnsButton size="small" className={classes.button} />
      <GridToolbarFilterButton componentsProps={{ button: { size: "small" } }} className={classes.button} />
      <Button className={classes.button} size="small" startIcon={<AddIcon />} onClick={onNewClick}>New</Button>
      {children}
    </GridToolbarContainer>
  );
};
