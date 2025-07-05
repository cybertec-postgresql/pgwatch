import { useCallback, useState } from 'react';
import { GridColDef, GridColumnVisibilityModel } from '@mui/x-data-grid';

export interface GridColumnSizingModel {
  [field: string]: number;
}

interface GridState {
  columnVisibility: GridColumnVisibilityModel;
  columnSizing: GridColumnSizingModel;
}

export const useGridState = (
  storageKey: string,
  columns: GridColDef[],
  defaultHidden: GridColumnVisibilityModel = {}
) => {
  const [gridState, setGridState] = useState<GridState>(() => {
    // Initialize default column visibility
    const defaultVisibility = columns?.reduce((acc, col) => ({
      ...acc,
      [col.field]: defaultHidden[col.field] === false ? false : true
    }), {});

    // Initialize default column sizing from column definitions
    const defaultSizing = columns?.reduce((acc, col) => ({
      ...acc,
      [col.field]: col.width || 150 // Use column width or default to 150
    }), {});

    // Load saved state from localStorage
    const saved = localStorage.getItem(storageKey);
    const savedState = saved ? JSON.parse(saved) : {};

    return {
      columnVisibility: {
        ...defaultVisibility,
        ...(savedState.columnVisibility || {})
      },
      columnSizing: {
        ...defaultSizing,
        ...(savedState.columnSizing || {})
      }
    };
  });

  const saveToStorage = useCallback((newState: GridState) => {
    localStorage.setItem(storageKey, JSON.stringify(newState));
  }, [storageKey]);

  const handleColumnVisibilityChange = useCallback((newModel: GridColumnVisibilityModel) => {
    const newState = {
      ...gridState,
      columnVisibility: newModel
    };
    setGridState(newState);
    saveToStorage(newState);
  }, [gridState, saveToStorage]);

  const handleColumnResize = useCallback((params: any) => {
    const newState = {
      ...gridState,
      columnSizing: {
        ...gridState.columnSizing,
        [params.field || params.colDef?.field]: params.width
      }
    };
    setGridState(newState);
    saveToStorage(newState);
  }, [gridState, saveToStorage]);

  const resetColumnSizes = useCallback(() => {
    const defaultSizing = columns?.reduce((acc, col) => ({
      ...acc,
      [col.field]: col.width || 150
    }), {});

    const newState = {
      ...gridState,
      columnSizing: defaultSizing
    };
    setGridState(newState);
    saveToStorage(newState);
  }, [columns, gridState, saveToStorage]);

  // Generate columns with applied widths
  const columnsWithSizing = columns?.map(col => ({
    ...col,
    width: gridState.columnSizing[col.field] || col.width || 150
  }));

  return {
    columnVisibility: gridState.columnVisibility,
    columnSizing: gridState.columnSizing,
    columnsWithSizing,
    onColumnVisibilityChange: handleColumnVisibilityChange,
    onColumnResize: handleColumnResize,
    resetColumnSizes
  };
};
