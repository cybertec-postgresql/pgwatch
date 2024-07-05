import { GridToolbar } from "components/GridToolbar/GridToolbar";
import { useSourceFormContext } from "contexts/SourceForm/SourceForm.context";
import { SourceFormActions } from "contexts/SourceForm/SourceForm.types";

export const SourcesGridToolbar = () => {
  const { handleOpen, setData, setAction } = useSourceFormContext();

  const onNewClick = () => {
    setData(undefined);
    setAction(SourceFormActions.Create);
    handleOpen();
  };

  return (
    <GridToolbar onNewClick={onNewClick} />
  );
};
