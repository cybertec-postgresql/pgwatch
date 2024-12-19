import { useState } from "react";
import { Source } from "types/Source/Source";
import { SourceFormContext } from "./SourceForm.context";
import { SourceFormActions, SourceFormContextType } from "./SourceForm.types";

type Props = {
  children?: React.ReactNode;
};

export const SourceFormProvider = ({ children }: Props) => {
  const [data, setData] = useState<Source | undefined>(undefined);
  const [action, setAction] = useState<SourceFormActions>(SourceFormActions.Create);
  const [open, setOpen] = useState(false);

  const handleOpen = () => { setOpen(true); };

  const handleClose = () => setOpen(false);

  const getContextValue = (): SourceFormContextType => ({
    data,
    setData,
    action,
    setAction,
    open,
    handleOpen,
    handleClose,
  });

  return (
    <SourceFormContext.Provider value={getContextValue()}>
      {children}
    </SourceFormContext.Provider>
  );
};
