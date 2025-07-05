import { useCallback, useState } from 'react';
import { GridColDef, GridColumnVisibilityModel } from '@mui/x-data-grid';

export const useGridColumnVisibility = (
  storageKey: string,
  columns: GridColDef[],
  defaultHidden: GridColumnVisibilityModel = {}
) => {
  const [columnVisibility, setColumnVisibility] = useState<GridColumnVisibilityModel>(() => {
    const defaultVisibility = columns?.reduce((acc, col) => ({
      ...acc,
      [col.field]: defaultHidden[col.field] === false ? false : true // Hidden if explicitly false, visible otherwise
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
