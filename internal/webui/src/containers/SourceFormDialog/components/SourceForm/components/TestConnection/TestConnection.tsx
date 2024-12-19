import CableIcon from '@mui/icons-material/Cable';
import { IconButton, Tooltip } from "@mui/material";
import { useTestConnection } from "queries/Source";

type Props = {
  ConnStr: string;
};

export const TestConnection = ({ ConnStr }: Props) => {
  const testConnection = useTestConnection();

  const handleTestConnection = () => {
    testConnection.mutate(ConnStr);
  };

  return (
    <Tooltip title="Test connection">
      <IconButton onClick={handleTestConnection} disabled={testConnection.isLoading}>
        <CableIcon />
      </IconButton>
    </Tooltip>
  );
};
