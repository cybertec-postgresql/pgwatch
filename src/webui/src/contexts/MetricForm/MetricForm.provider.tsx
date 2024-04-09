import { useState } from "react";
import { MetricGridRow } from "pages/MetricsPage/components/MetricsGrid/MetricsGrid.types";
import { MetricFormContext } from "./MetricForm.context";
import { MetricFormContextType } from "./MetricForm.types";

type Props = {
  open: boolean;
  handleOpen: () => void;
  handleClose: () => void;
  children?: React.ReactNode;
};

export const MetricFormProvider = (props: Props) => {
  const { open, handleOpen, handleClose, children } = props;
  const [data, setData] = useState<MetricGridRow | undefined>(undefined);

  const getContextValue = (): MetricFormContextType => ({
    data,
    setData,
    open,
    handleOpen,
    handleClose,
  });

  return (
    <MetricFormContext.Provider value={getContextValue()}>
      {children}
    </MetricFormContext.Provider>
  );
};
