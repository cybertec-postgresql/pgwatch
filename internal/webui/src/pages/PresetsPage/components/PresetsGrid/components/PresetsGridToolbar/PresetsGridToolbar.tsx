import { GridToolbar } from "components/GridToolbar/GridToolbar";
import { usePresetFormContext } from "contexts/PresetForm/PresetForm.context";

type Props = {
  onResetColumns?: () => void;
};

export const PresetsGridToolbar = ({ onResetColumns }: Props) => {
  const { handleOpen, setData } = usePresetFormContext();

  const onNewClick = () => {
    setData(undefined);
    handleOpen();
  };

  return (
    <GridToolbar onNewClick={onNewClick} onResetColumns={onResetColumns} />
  );
};
