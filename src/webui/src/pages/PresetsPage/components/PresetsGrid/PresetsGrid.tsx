import { useMemo, useState } from "react";
import { DataGrid } from "@mui/x-data-grid";
import { PresetFormDialog } from "containers/PresetFormDialog/PresetFormDialog";
import { PresetFormProvider } from "contexts/PresetForm/PresetForm.provider";
import { usePageStyles } from "styles/page";
import { ErrorComponent } from "layout/common/ErrorComponent";
import { LoadingComponent } from "layout/common/LoadingComponent";
import { usePresets } from "queries/Preset";
import { usePresetsGridColumns } from "./PresetsGrid.consts";
import { PresetGridRow } from "./PresetsGrid.types";
import { PresetsGridToolbar } from "./components/PresetsGridToolbar/PresetsGridToolbar";

export const PresetsGrid = () => {
  const [formDialogOpen, setFormDialogOpen] = useState(false);

  const { classes } = usePageStyles();

  const { status, data, error } = usePresets();

  const rows: PresetGridRow[] | [] = useMemo(() => {
    if (data) {
      return Object.keys(data).map((key) => {
        const preset = data[key];
        return {
          Key: key,
          Preset: preset,
        };
      });
    }
    return [];
  }, [data]);

  const columns = usePresetsGridColumns();

  const handleFormDialogOpen = () => setFormDialogOpen(true);

  const handleFormDialogClose = () => setFormDialogOpen(false);

  if (status === "loading") {
    return (
      <LoadingComponent />
    );
  };

  if (status === "error") {
    const err = error as Error;
    return (
      <ErrorComponent errorMessage={err.message} />
    );
  };

  return (
    <div className={classes.page}>
      <PresetFormProvider
        open={formDialogOpen}
        handleOpen={handleFormDialogOpen}
        handleClose={handleFormDialogClose}
      >
        <DataGrid
          getRowId={(row) => row.Key}
          columns={columns}
          rows={rows}
          rowsPerPageOptions={[]}
          components={{ Toolbar: () => <PresetsGridToolbar /> }}
          disableColumnMenu
        />
        <PresetFormDialog />
      </PresetFormProvider>
    </div>
  );
};
