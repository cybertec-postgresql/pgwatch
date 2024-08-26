import { useEffect, useMemo, useState } from "react";
import { GridActions } from "components/GridActions/GridActions";
import { WarningDialog } from "components/WarningDialog/WarningDialog";
import { usePresetFormContext } from "contexts/PresetForm/PresetForm.context";
import { useDeletePreset } from "queries/Preset";
import { PresetGridRow } from "../../PresetsGrid.types";

type Props = {
  preset: PresetGridRow;
};

export const PresetsGridActions = ({ preset }: Props) => {
  const [dialogOpen, setDialogOpen] = useState(false);
  const { setData, handleOpen } = usePresetFormContext();
  const { mutate, isSuccess } = useDeletePreset();

  const handleDialogClose = () => setDialogOpen(false);

  const handleEditClick = () => {
    setData(preset);
    handleOpen();
  };

  const handleDeleteClick = () => setDialogOpen(true);

  const handleSubmit = () => mutate(preset.Key);

  const message = useMemo(
    () => `Are you sure want to delete preset "${preset.Key}"`,
    [preset],
  );

  useEffect(() => {
    isSuccess && handleDialogClose();
  }, [isSuccess]);

  return (
    <>
      <GridActions handleEditClick={handleEditClick} handleDeleteClick={handleDeleteClick} />
      <WarningDialog open={dialogOpen} message={message} onClose={handleDialogClose} onSubmit={handleSubmit} />
    </>
  );
};
