import AddIcon from "@mui/icons-material/Add";
import { Button as MuiButton } from '@mui/material';
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
      <MuiButton startIcon={<AddIcon />} onClick={onNewClick}>New</MuiButton>
      {children}
    </GridToolbarContainer>
  );
};
