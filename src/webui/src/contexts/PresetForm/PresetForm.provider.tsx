import { useState } from "react";
import { PresetGridRow } from "pages/PresetsPage/components/PresetsGrid/PresetsGrid.types";
import { PresetFormContext } from "./PresetForm.context";
import { PresetFormContextType } from "./PresetForm.types";

type Props = {
  open: boolean;
  handleOpen: () => void;
  handleClose: () => void;
  children?: React.ReactNode;
};

export const PresetFormProvider = (props: Props) => {
  const { open, handleOpen, handleClose, children } = props;
  const [data, setData] = useState<PresetGridRow | undefined>(undefined);

  const getContextValue = (): PresetFormContextType => ({
    data,
    setData,
    open,
    handleOpen,
    handleClose,
  });

  return(
    <PresetFormContext.Provider value={getContextValue()}>
      {children}
    </PresetFormContext.Provider>
  );
};
