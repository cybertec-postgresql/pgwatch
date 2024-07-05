import { useState } from "react";
import { Source } from "types/Source/Source";
import { SourceFormContext } from "./SourceForm.context";
import { SourceFormActions, SourceFormContextType } from "./SourceForm.types";

type Props = {
  open: boolean;
  handleOpen: () => void;
  handleClose: () => void;
  children?: React.ReactNode;
};

export const SourceFormProvider = (props: Props) => {
  const { open, handleOpen, handleClose, children } = props;
  const [data, setData] = useState<Source | undefined>(undefined);
  const [action, setAction] = useState<SourceFormActions>(SourceFormActions.Create);

  const getContextValue = (): SourceFormContextType => ({
    data,
    setData,
    action,
    setAction,
    open,
    handleOpen,
    handleClose,
  });

  return(
    <SourceFormContext.Provider value={getContextValue()}>
      {children}
    </SourceFormContext.Provider>
  );
};
