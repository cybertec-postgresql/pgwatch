import { PRESET_ORDER } from "constants/presets";
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

  const columns = usePresetsGridColumns();
  const { 
    columnVisibility, 
    columnsWithSizing, 
    onColumnVisibilityChange, 
    onColumnResize
  } = useGridState('PRESETS_GRID', columns);

  const rows: PresetGridRow[] = useMemo(() => {
  if (!data) {
    return [];
  }
  const orderMap = new Map(
    PRESET_ORDER.map((name, index) => [name, index])
  );

  return Object.keys(data)
    .sort((a, b) => {
      const aOrder = orderMap.get(a) ?? Number.MAX_SAFE_INTEGER;
      const bOrder = orderMap.get(b) ?? Number.MAX_SAFE_INTEGER;
      return aOrder - bOrder;
    })
    .map((key) => ({
      Key: key,
      Preset: data[key],
    }));
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
          columns={columnsWithSizing}
          rows={rows}
          pageSizeOptions={[]}
          slots={{ 
            toolbar: () => <PresetsGridToolbar />
          }}
          columnVisibilityModel={columnVisibility}
          onColumnVisibilityModelChange={onColumnVisibilityChange}
          onColumnResize={onColumnResize}
        />
        <PresetFormDialog />
      </PresetFormProvider>
    </div>
  );
};
