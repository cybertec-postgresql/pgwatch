import { useEffect, useState } from "react";
import { GridActions } from "components/GridActions/GridActions";
import { WarningDialog } from "components/WarningDialog/WarningDialog";
import { useMetricFormContext } from "contexts/MetricForm/MetricForm.context";
import { useDeleteMetric } from "queries/Metric";
import { MetricGridRow } from "../../MetricsGrid.types";

type Props = {
  metric: MetricGridRow;
};

export const MetricsGridActions = ({ metric }: Props) => {
  const { setData, handleOpen } = useMetricFormContext();
  const [dialogOpen, setDialogOpen] = useState(false);
  const { mutate, isSuccess } = useDeleteMetric();

  useEffect(() => {
    isSuccess && handleDialogClose();
  }, [isSuccess]);

  const handleDialogClose = () => setDialogOpen(false);

  const handleEditClick = () => {
    setData(metric);
    handleOpen();
  };

  const handleDeleteClick = () => setDialogOpen(true);

  const handleSubmit = () => mutate(metric.Key);

  const message = `Are you sure want to delete metric "${metric.Key}"`;

  return (
    <>
      <GridActions handleEditClick={handleEditClick} handleDeleteClick={handleDeleteClick} />
      <WarningDialog open={dialogOpen} message={message} onClose={handleDialogClose} onSubmit={handleSubmit} />
    </>
  );
};
