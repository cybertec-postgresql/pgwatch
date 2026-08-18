import { useCallback, useMemo, useState } from 'react';
import { GridColDef, GridColumnVisibilityModel } from '@mui/x-data-grid';

interface GridState {
  columnVisibility?: GridColumnVisibilityModel;
}

const readSavedState = (storageKey: string): GridState => {
  const saved = localStorage.getItem(storageKey);
  return saved ? JSON.parse(saved) : {};
};

const persistState = (storageKey: string, state: GridState) => {
  const current = readSavedState(storageKey);
  localStorage.setItem(storageKey, JSON.stringify({ ...current, ...state }));
};

export const useGridState = (
  storageKey: string,
  columns: GridColDef[],
  defaultHidden: GridColumnVisibilityModel = {}
) => {
  // Column widths are intentionally left as uncontrolled defaults (from the
  // column definitions) and are NOT persisted. This lets the DataGrid fully
  // own the resize interaction, which avoids the glitchy/jumpy resizing caused
  // by controlling widths, and resets column widths on each session.
  const [columnVisibility, setColumnVisibility] = useState<GridColumnVisibilityModel>(() => {
    const saved = readSavedState(storageKey);
    const defaultVisibility = columns?.reduce((acc, col) => ({
      ...acc,
      [col.field]: defaultHidden[col.field] === false ? false : true
    }), {} as GridColumnVisibilityModel);

    return {
      ...defaultVisibility,
      ...(saved.columnVisibility || {})
    };
  });

  const handleColumnVisibilityChange = useCallback((newModel: GridColumnVisibilityModel) => {
    setColumnVisibility(newModel);
    persistState(storageKey, { columnVisibility: newModel });
  }, [storageKey]);

  // Make the last visible column stretch to fill the remaining horizontal
  // space, so the grid always spans the full width of the screen. The "last"
  // column is determined dynamically from the current visibility, so it stays
  // correct when columns are hidden/shown via the filters.
  const columnsWithFill = useMemo(() => {
    const fillField = [...columns]
      .reverse()
      .find((col) => columnVisibility[col.field] !== false)
      ?.field;

    return columns.map((col) => {
      if (col.field !== fillField) {
        return col;
      }
      return {
        ...col,
        flex: col.flex ?? 1,
        minWidth: col.minWidth ?? col.width ?? 150,
      };
    });
  }, [columns, columnVisibility]);

  return {
    columnVisibility,
    columns: columnsWithFill,
    onColumnVisibilityChange: handleColumnVisibilityChange,
  };
};
