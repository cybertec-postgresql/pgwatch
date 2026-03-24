import { GridToolbar } from "components/GridToolbar/GridToolbar";
import { useMetricFormContext } from "contexts/MetricForm/MetricForm.context";

export const MetricsGridToolbar = ({ onResetColumns }: { onResetColumns: () => void }) => {
  const { handleOpen, setData } = useMetricFormContext();

  type Props = {
    onResetColumns: () => void;
  };

  const onNewClick = () => {
    setData(undefined);
    handleOpen();
  };

  return (
    <GridToolbar onNewClick={onNewClick} onResetColumns={onResetColumns} />
  );
};
