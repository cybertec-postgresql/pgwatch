import { MetricGridRow } from "pages/MetricsPage/components/MetricsGrid/MetricsGrid.types";

export type MetricFormContextType = {
  data: MetricGridRow | undefined;
  setData: React.Dispatch<React.SetStateAction<MetricGridRow | undefined>>;
  open: boolean;
  handleOpen: () => void;
  handleClose: () => void;
};
