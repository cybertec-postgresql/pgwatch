import { useState } from "react";
import { PresetGridRow } from "pages/PresetsPage/components/PresetsGrid/PresetsGrid.types";
import { PresetFormContext } from "./PresetForm.context";
import { PresetFormContextType } from "./PresetForm.types";

type Props = {
  children?: React.ReactNode;
};

export const PresetFormProvider = ({ children }: Props) => {
  const [data, setData] = useState<PresetGridRow | undefined>(undefined);
  const [open, setOpen] = useState(false);

  const handleOpen = () => setOpen(true);

  const handleClose = () => setOpen(false);

  const getContextValue = (): PresetFormContextType => ({
    data,
    setData,
    open,
    handleOpen,
    handleClose,
  });

  return (
    <PresetFormContext.Provider value={getContextValue()}>
      {children}
    </PresetFormContext.Provider>
  );
};
