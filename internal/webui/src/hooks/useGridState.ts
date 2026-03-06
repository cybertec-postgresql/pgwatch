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

    // Load saved state from localStorage
    const saved = localStorage.getItem(storageKey);
    const savedState = saved ? JSON.parse(saved) : {};

    return {
     columnVisibility: {
        ...defaultVisibility,
        ...(savedState.columnVisibility || {})
      },
      columnSizing: savedState.columnSizing || {}
    };
  });

  const handleColumnVisibilityChange = useCallback((newModel: GridColumnVisibilityModel) => {
   setGridState(prev => {
      const newState = {
        ...prev,
        columnSizing: {
          ...prev.columnSizing,
          [params.colDef?.field ?? params.field]: params.width
        }
      };
      localStorage.setItem(storageKey, JSON.stringify(newState));
      return newState;
    });
  }, [storageKey]);

  const handleColumnWidthChange = useCallback((params: any) => {
    setGridState(prev => {
      const newState = {
        ...prev,
        columnSizing: {
          ...prev.columnSizing,
          [params.colDef?.field ?? params.field]: params.width
        }
      };
      localStorage.setItem(storageKey, JSON.stringify(newState));
      return newState;
    });
  }, [storageKey]);

 const resetColumnSizes = useCallback(() => {
    setGridState(prev => {
      const newState = { ...prev, columnSizing: {} };
      localStorage.setItem(storageKey, JSON.stringify(newState));
      return newState;
    });
  }, [storageKey]);

  const columnsWithSizing = useMemo(() =>
    columns?.map(col => {
      const userWidth = gridState.columnSizing[col.field];
      if (userWidth !== undefined) {
        const { flex, ...colWithoutFlex } = col as any;
        return { ...colWithoutFlex, width: userWidth };
      }
      return col;
    }),
    [columns, gridState.columnSizing]
  );

  return {
    columnVisibility: gridState.columnVisibility,
    columnSizing: gridState.columnSizing,
    columnsWithSizing,
    onColumnVisibilityChange: handleColumnVisibilityChange,
    onColumnWidthChange: handleColumnWidthChange,
    resetColumnSizes
  };
};
