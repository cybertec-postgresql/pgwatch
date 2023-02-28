import AddIcon from '@mui/icons-material/Add';
import { Button } from "@mui/material";
import { GridToolbarColumnsButton, GridToolbarContainer, GridToolbarFilterButton } from "@mui/x-data-grid";

type Props = {
  handleModalOpen: () => void,
  setEditData: React.Dispatch<React.SetStateAction<any>>
}

export const GridToolbarComponent = ({ handleModalOpen, setEditData }: Props) => {

  const handleNewClick = () => {
    handleModalOpen();
    setEditData(undefined);
  };

  return (
    <GridToolbarContainer>
      <GridToolbarColumnsButton size="small" sx={{ height: 32 }} />
      <GridToolbarFilterButton componentsProps={{ button: { size: "small" } }} sx={{ height: 32 }} />
      <Button onClick={handleNewClick} size="small" startIcon={<AddIcon />} sx={{ height: 32 }}>New</Button>
    </GridToolbarContainer>
  );
};
