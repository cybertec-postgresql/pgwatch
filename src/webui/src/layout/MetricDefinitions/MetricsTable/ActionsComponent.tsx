import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import VisibilityIcon from '@mui/icons-material/Visibility';
import { Box, Button } from "@mui/material";


export const ActionsComponent = () => {


  return (
    <Box sx={{ display: "flex", justifyContent: "space-around", width: "100%" }}>
      <Button
        size="small"
        variant="outlined"
        startIcon={<VisibilityIcon />}
      >
        Show SQL
      </Button>
      <Button
        size="small"
        variant="outlined"
        startIcon={<EditIcon />}
      >
        Edit
      </Button>
      <Button
        size="small"
        variant="contained"
        startIcon={<DeleteIcon />}
      >
        Delete
      </Button>
    </Box>
  );
};
