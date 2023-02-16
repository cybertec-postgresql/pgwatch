import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import { Box, Button } from "@mui/material";

type Props = {
  data: any
};

export const ActionsComponent = ({ data }: Props) => {

  const handleClick = () => {
    alert(JSON.stringify(data));
  };

  return (
    <Box sx={{ display: "flex", justifyContent: "space-between", width: "100%" }}>
      <Button
        size="small"
        variant="outlined"
        startIcon={<EditIcon />}
        onClick={handleClick}
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
