import { GridToolbar } from "components/GridToolbar/GridToolbar";
import { useMetricFormContext } from "contexts/MetricForm/MetricForm.context";

export const MetricsGridToolbar = () => {
  const { handleOpen, setData } = useMetricFormContext();

  const onNewClick = () => {
    setData(undefined);
    handleOpen();
  };

  return (
    <GridToolbar onNewClick={onNewClick} />
  );
};
