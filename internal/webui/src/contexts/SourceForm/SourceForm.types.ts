import { Source } from "types/Source/Source";

export enum SourceFormActions {
  Create = "Create",
  Edit = "Edit",
  Copy = "Copy",
};

export type SourceFormContextType = {
  data: Source | undefined;
  setData: React.Dispatch<React.SetStateAction<Source | undefined>>;
  action: SourceFormActions;
  setAction: React.Dispatch<React.SetStateAction<SourceFormActions>>;
  open: boolean;
  handleOpen: () => void;
  handleClose: () => void;
};
