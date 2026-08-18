import { useMemo } from "react";
import { DataGrid } from "@mui/x-data-grid";
import { Error } from "components/Error/Error";
import { Loading } from "components/Loading/Loading";
import { PresetFormDialog } from "containers/PresetFormDialog/PresetFormDialog";
import { PresetFormProvider } from "contexts/PresetForm/PresetForm.provider";
import { useGridState } from 'hooks/useGridState';
import { usePageStyles } from "styles/page";
import { usePresets } from "queries/Preset";
import { usePresetsGridColumns } from "./PresetsGrid.consts";
import { PresetGridRow } from "./PresetsGrid.types";
import { PresetsGridToolbar } from "./components/PresetsGridToolbar/PresetsGridToolbar";

export const PresetsGrid = () => {
  const { classes } = usePageStyles();

  const { data, isLoading, isError, error } = usePresets();

  const gridColumns = usePresetsGridColumns();
  const { 
    columnVisibility, 
    columns,
    onColumnVisibilityChange, 
  } = useGridState('PRESETS_GRID', gridColumns);

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

  if (isLoading) {
    return (
      <Loading />
    );
  };

  if (isError) {
    const err = error as Error;
    return (
      <Error message={err.message} />
    );
  };

  return (
    <div className={classes.page}>
      <PresetFormProvider>
        <DataGrid
          getRowId={(row) => row.Key}
          columns={columns}
          rows={rows}
          pageSizeOptions={[]}
          showToolbar
          slots={{ 
            toolbar: () => <PresetsGridToolbar />
          }}
          columnVisibilityModel={columnVisibility}
          onColumnVisibilityModelChange={onColumnVisibilityChange}
        />
        <PresetFormDialog />
      </PresetFormProvider>
    </div>
  );
};
