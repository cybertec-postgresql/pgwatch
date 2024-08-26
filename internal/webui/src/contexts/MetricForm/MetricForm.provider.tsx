import { useState } from "react";
import { MetricGridRow } from "pages/MetricsPage/components/MetricsGrid/MetricsGrid.types";
import { MetricFormContext } from "./MetricForm.context";
import { MetricFormContextType } from "./MetricForm.types";

type Props = {
  children?: React.ReactNode;
};

export const MetricFormProvider = ({ children }: Props) => {
  const [data, setData] = useState<MetricGridRow | undefined>(undefined);
  const [open, setOpen] = useState(false);

  const handleOpen = () => setOpen(true);

  const handleClose = () => setOpen(false);

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
