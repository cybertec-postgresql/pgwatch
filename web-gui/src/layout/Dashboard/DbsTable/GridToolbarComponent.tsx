import AddIcon from '@mui/icons-material/Add';
import { Button } from "@mui/material";
import { GridToolbarColumnsButton, GridToolbarContainer, GridToolbarFilterButton } from "@mui/x-data-grid";

export const GridToolbarComponent = (setModalOpen: any, setEditData: any) => {

    return (
        <GridToolbarContainer>
            <GridToolbarColumnsButton size="small" sx={{ height: 32, marginRight: "5px" }} />
            <GridToolbarFilterButton componentsProps={{ button: { size: "small" } }} sx={{ height: 32, marginRight: "5px" }} />
            <Button onClick={() => { setModalOpen(true); setEditData(undefined); }} size="small" startIcon={<AddIcon />} sx={{ height: 32 }}>New</Button>
        </GridToolbarContainer>
    );
};
