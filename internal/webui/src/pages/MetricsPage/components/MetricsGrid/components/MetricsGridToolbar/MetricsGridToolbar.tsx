import { GridToolbar } from "components/GridToolbar/GridToolbar";
import { useMetricFormContext } from "contexts/MetricForm/MetricForm.context";

type Props = {
  onResetColumns: () => void;
};

export const MetricsGridToolbar = ({ onResetColumns }: Props) => {
  const { handleOpen, setData } = useMetricFormContext();

  const onNewClick = () => {
    setData(undefined);
    handleOpen();
  };

  return (
    <GridToolbar onNewClick={onNewClick} onResetColumns={onResetColumns} />
  );
};
