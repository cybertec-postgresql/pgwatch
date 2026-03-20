import AddIcon from "@mui/icons-material/Add";
import RestartAltIcon from "@mui/icons-material/RestartAlt";
import { Button } from '@mui/material';
import { GridToolbarColumnsButton, GridToolbarContainer, GridToolbarFilterButton } from "@mui/x-data-grid";

type Props = {
  onNewClick: () => void;
  onResetColumns?: () => void;
  children?: React.ReactNode;
};

export const GridToolbar = (props: Props) => {
  const { onNewClick, onResetColumns, children } = props;

  return (
    <GridToolbarContainer>
      <GridToolbarColumnsButton slotProps={{ button: { size: "small" } }} />
      <GridToolbarFilterButton slotProps={{ button: { size: "small" } }} />
      <Button startIcon={<AddIcon />} onClick={onNewClick}>New</Button>
      {onResetColumns && (
        <Button startIcon={<RestartAltIcon />} onClick={onResetColumns}>
          Reset Columns
        </Button>
      )}
      {children}
    </GridToolbarContainer>
  );
};
