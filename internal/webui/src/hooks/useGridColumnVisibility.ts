import { useCallback, useState } from 'react';
import { GridColDef, GridColumnVisibilityModel } from '@mui/x-data-grid';

export const useGridColumnVisibility = (
  storageKey: string,
  columns: GridColDef[]
) => {
  const [columnVisibility, setColumnVisibility] = useState<GridColumnVisibilityModel>(() => {
    const defaultVisibility = columns?.reduce((acc, col) => ({
      ...acc,
      [col.field]: col.hide !== true
    }), {});

    const saved = localStorage.getItem(storageKey);
    const savedVisibility = saved ? JSON.parse(saved) : {};

    return {
      ...defaultVisibility,
      ...savedVisibility
    };
  });

  const handleColumnVisibilityChange = useCallback((newModel: GridColumnVisibilityModel) => {
    setColumnVisibility(newModel);
    localStorage.setItem(storageKey, JSON.stringify(newModel));
  }, [storageKey]);

  return {
    columnVisibility,
    onColumnVisibilityChange: handleColumnVisibilityChange
  };
};
