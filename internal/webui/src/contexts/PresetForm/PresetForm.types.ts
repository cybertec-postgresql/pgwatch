import { PresetGridRow } from "pages/PresetsPage/components/PresetsGrid/PresetsGrid.types";

export type PresetFormContextType = {
  data: PresetGridRow | undefined;
  setData: React.Dispatch<React.SetStateAction<PresetGridRow | undefined>>;
  open: boolean;
  handleOpen: () => void;
  handleClose: () => void;
};
