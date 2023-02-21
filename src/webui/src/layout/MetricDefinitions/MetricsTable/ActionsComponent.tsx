import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import { AlertColor, Box, Button } from "@mui/material";
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { QueryKeys } from 'queries/queryKeys';
import { Metric } from 'queries/types/MetricTypes';
import MetricService from 'services/Metric';

type Params = {
  data: Metric,
  handleAlertOpen: (isOpen: boolean, text: string, type: AlertColor) => void,
  setEditData: React.Dispatch<React.SetStateAction<Metric | undefined>>,
  handleModalOpen: () => void
};

export const ActionsComponent = ({ data, handleAlertOpen, setEditData, handleModalOpen }: Params) => {
  const services = MetricService.getInstance();
  const queryClient = useQueryClient();

  const deleteRecord = useMutation({
    mutationFn: async (m_id: number) => {
      return await services.deleteMetric(m_id);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: QueryKeys.metric });
      handleAlertOpen(true, `Metric "${data.m_name}" has been deleted successfully!`, "success");
    },
    onError: (error: any) => {
      handleAlertOpen(true, error.response.data, "error");
    }
  });

  const handleEditClick = () => {
    setEditData(data);
    handleModalOpen();
  };

  const handleDeleteClick = () => {
    deleteRecord.mutate(data.m_id);
  };

  return (
    <Box sx={{ display: "flex", justifyContent: "space-between", width: "100%" }}>
      <Button
        size="small"
        variant="outlined"
        startIcon={<EditIcon />}
        onClick={handleEditClick}
      >
        Edit
      </Button>
      <Button
        size="small"
        variant="contained"
        startIcon={<DeleteIcon />}
        onClick={handleDeleteClick}
      >
        Delete
      </Button>
    </Box>
  );
};
