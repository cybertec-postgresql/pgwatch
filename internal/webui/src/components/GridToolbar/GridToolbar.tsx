import AddIcon from "@mui/icons-material/Add";
import { Button } from '@mui/material';
import { GridToolbarColumnsButton, GridToolbarContainer, GridToolbarFilterButton } from "@mui/x-data-grid";

type Props = {
  onNewClick: () => void;
  children?: React.ReactNode;
};

export const GridToolbar = (props: Props) => {
  const { onNewClick, children } = props;

  return (
    <GridToolbarContainer>
      <GridToolbarColumnsButton size="small" />
      <GridToolbarFilterButton componentsProps={{ button: { size: "small" } }} />
      <Button startIcon={<AddIcon />} onClick={onNewClick}>New</Button>
      {children}
    </GridToolbarContainer>
  );
};
