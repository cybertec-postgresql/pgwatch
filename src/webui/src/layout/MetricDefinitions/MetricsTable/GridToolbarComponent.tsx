import AddIcon from '@mui/icons-material/Add';
import { Button } from "@mui/material";
import { GridToolbarColumnsButton, GridToolbarContainer, GridToolbarFilterButton } from "@mui/x-data-grid";


export const GridToolbarComponent = () => {


  return (
    <GridToolbarContainer>
      <GridToolbarColumnsButton size="small" sx={{ height: 32 }} />
      <GridToolbarFilterButton componentsProps={{ button: { size: "small" } }} sx={{ height: 32 }} />
      <Button size="small" startIcon={<AddIcon />} sx={{ height: 32 }}>New</Button>
    </GridToolbarContainer>
  );
};
