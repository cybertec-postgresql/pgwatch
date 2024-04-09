import DeleteIcon from "@mui/icons-material/Delete";
import EditIcon from "@mui/icons-material/Edit";
import { IconButton } from "@mui/material";
import { Root } from "./GridActions.styles";

type Props = {
  handleEditClick: () => void;
  handleDeleteClick: () => void;
  children?: React.ReactNode;
};

export const GridActions = (props: Props) => {
  const { children, handleDeleteClick, handleEditClick } = props;

  return (
    <Root>
      <IconButton title="Edit" onClick={handleEditClick}>
        <EditIcon />
      </IconButton>
      <IconButton title="Delete" onClick={handleDeleteClick}>
        <DeleteIcon />
      </IconButton>
      {children}
    </Root>
  );
};
