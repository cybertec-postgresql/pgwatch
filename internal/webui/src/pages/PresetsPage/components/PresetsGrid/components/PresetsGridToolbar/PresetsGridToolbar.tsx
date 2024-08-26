import { GridToolbar } from "components/GridToolbar/GridToolbar";
import { usePresetFormContext } from "contexts/PresetForm/PresetForm.context";

export const PresetsGridToolbar = () => {
  const { handleOpen, setData } = usePresetFormContext();

  const onNewClick = () => {
    setData(undefined);
    handleOpen();
  };

  return (
    <GridToolbar onNewClick={onNewClick} />
  );
};
