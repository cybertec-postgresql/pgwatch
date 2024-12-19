import { SourceFormActions, SourceFormContextType } from "./SourceForm.types";

export const SourceFormContextDefaults: SourceFormContextType = {
  data: undefined,
  setData: () => undefined,
  action: SourceFormActions.Create,
  setAction: () => undefined,
  open: false,
  handleOpen: () => undefined,
  handleClose: () => undefined,
};
