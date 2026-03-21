import { useCallback, useMemo, useState } from 'react';
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
    setGridState(prev => {
      const newState = { ...prev, columnVisibility: newModel };
      saveToStorage(newState);
      return newState;
    });
  }, [saveToStorage]);

  const handleColumnWidthChange = useCallback((params: any) => {
    setGridState(prev => {
      const newState = {
        ...prev,
        columnSizing: {
          ...prev.columnSizing,
          [params.field || params.colDef?.field]: params.width
        }
      };
      saveToStorage(newState);
      return newState;
    });
  }, [saveToStorage]);

  const resetColumnSizes = useCallback(() => {
    const defaultSizing = columns?.reduce((acc, col) => ({
      ...acc,
      [col.field]: col.width || 150
    }), {});

    setGridState(prev => {
      const newState = { ...prev, columnSizing: defaultSizing };
      saveToStorage(newState);
      return newState;
    });
  }, [columns, saveToStorage]);

  // Memoize columns with applied widths so objects are stable between renders
  const columnsWithSizing = useMemo(() => columns?.map(col => ({
    ...col,
    width: gridState.columnSizing[col.field] || col.width || 150
  })), [columns, gridState.columnSizing]);

  return {
    columnVisibility: gridState.columnVisibility,
    columnSizing: gridState.columnSizing,
    columnsWithSizing,
    onColumnVisibilityChange: handleColumnVisibilityChange,
    onColumnWidthChange: handleColumnWidthChange,
    resetColumnSizes
  };
};
