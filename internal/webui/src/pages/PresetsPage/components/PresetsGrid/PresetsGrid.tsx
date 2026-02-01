import { useMemo, useState, useEffect, useCallback } from "react";
import { DataGrid, GridColDef } from "@mui/x-data-grid";
import { Error as ErrorComponent } from "components/Error/Error";
import { Loading } from "components/Loading/Loading";
import { PresetFormDialog } from "containers/PresetFormDialog/PresetFormDialog";
import { PresetFormProvider } from "contexts/PresetForm/PresetForm.provider";
import { useGridState } from "hooks/useGridState";
import { usePageStyles } from "styles/page";
import { usePresets, useReorderPresets } from "queries/Preset";
import { usePresetsGridColumns } from "./PresetsGrid.consts";
import { PresetGridRow } from "./PresetsGrid.types";
import { PresetsGridToolbar } from "./components/PresetsGridToolbar/PresetsGridToolbar";
import { useQueryClient } from "@tanstack/react-query";
import { QueryKeys } from "consts/queryKeys";
import { Box, IconButton } from "@mui/material";
import ArrowUpwardIcon from "@mui/icons-material/ArrowUpward";
import ArrowDownwardIcon from "@mui/icons-material/ArrowDownward";

export const PresetsGrid = () => {
  const { classes } = usePageStyles();
  const queryClient = useQueryClient();
  const { data, isLoading, isError, error } = usePresets();
  const reorderPresets = useReorderPresets();

  const [orderedRows, setOrderedRows] = useState<PresetGridRow[]>([]);

  // Initialize ordered rows when data changes
  useEffect(() => {
    if (data) {
      const rows = Object.entries(data)
        .sort(([, a], [, b]) => a.SortOrder - b.SortOrder)
        .map(([key, preset]) => ({
          Key: key,
          Preset: preset,
        }));
      setOrderedRows(rows);
    }
  }, [data]);

  const handleMoveRow = useCallback(
    (rowKey: string, direction: "up" | "down") => {
      const currentIndex = orderedRows.findIndex((r) => r.Key === rowKey);
      if (currentIndex === -1) return;

      const newIndex = direction === "up" ? currentIndex - 1 : currentIndex + 1;
      if (newIndex < 0 || newIndex >= orderedRows.length) return;

      // reorder rows
      const newOrderedRows = Array.from(orderedRows);
      const [removed] = newOrderedRows.splice(currentIndex, 1);
      newOrderedRows.splice(newIndex, 0, removed);

      // optimistic update
      setOrderedRows(newOrderedRows);

      const updatedPresets = newOrderedRows.map((row, index) => ({
        Name: row.Key,
        Data: {
          Description: row.Preset.Description,
          Metrics: row.Preset.Metrics,
          SortOrder: index,
        },
      }));

      reorderPresets.mutate(updatedPresets, {
        onSuccess: () => {
          // Invalidate the query to refresh the data
          queryClient.invalidateQueries({ queryKey: [QueryKeys.Preset] });
        },
        onError: () => {
          // Revert to original order on error
          if (data) {
            const rows = Object.entries(data)
              .sort(([, a], [, b]) => a.SortOrder - b.SortOrder)
              .map(([key, preset]) => ({
                Key: key,
                Preset: preset,
              }));
            setOrderedRows(rows);
          }
        },
      });
    },
    [orderedRows, data, queryClient, reorderPresets]
  );

  // columns with reorder controls
  const baseColumns = usePresetsGridColumns();
  const columns: GridColDef<PresetGridRow>[] = useMemo(() => {
    const reorderColumn: GridColDef<PresetGridRow> = {
      field: "reorder",
      headerName: "Order",
      width: 80,
      sortable: false,
      filterable: false,
      hideable: false,
      disableColumnMenu: true,
      align: "center",
      headerAlign: "center",
      renderCell: (params) => {
        const index = orderedRows.findIndex((r) => r.Key === params.row.Key);
        const isFirst = index === 0;
        const isLast = index === orderedRows.length - 1;

        return (
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              gap: 0.5,
              width: "100%",
              height: "100%",
            }}
          >
            <IconButton
              size="small"
              disabled={isFirst || reorderPresets.isPending}
              onClick={(e) => {
                e.stopPropagation();
                handleMoveRow(params.row.Key, "up");
              }}
              sx={{
                p: 0.25,
                "&.Mui-disabled": {
                  opacity: 0.3,
                },
              }}
            >
              <ArrowUpwardIcon fontSize="small" />
            </IconButton>
            <IconButton
              size="small"
              disabled={isLast || reorderPresets.isPending}
              onClick={(e) => {
                e.stopPropagation();
                handleMoveRow(params.row.Key, "down");
              }}
              sx={{
                p: 0.25,
                "&.Mui-disabled": {
                  opacity: 0.3,
                },
              }}
            >
              <ArrowDownwardIcon fontSize="small" />
            </IconButton>
          </Box>
        );
      },
    };
    return [reorderColumn, ...baseColumns];
  }, [baseColumns, orderedRows, reorderPresets.isPending, handleMoveRow]);

  const {
    columnVisibility,
    columnsWithSizing,
    onColumnVisibilityChange,
    onColumnResize,
  } = useGridState("PRESETS_GRID", columns);

  if (isLoading) {
    return <Loading />;
  }

  if (isError) {
    const err = error as Error;
    return <ErrorComponent message={err.message} />;
  }

  return (
    <div className={classes.page}>
      <PresetFormProvider>
        <DataGrid
          getRowId={(row) => row.Key}
          columns={columnsWithSizing}
          rows={orderedRows}
          pageSizeOptions={[]}
          slots={{
            toolbar: PresetsGridToolbar,
          }}
          columnVisibilityModel={columnVisibility}
          onColumnVisibilityModelChange={onColumnVisibilityChange}
          onColumnResize={onColumnResize}
          disableRowSelectionOnClick
          sx={{
            "& .MuiDataGrid-cell:focus": {
              outline: "none",
            },
            "& .MuiDataGrid-cell:focus-within": {
              outline: "none",
            },
          }}
        />
        <PresetFormDialog />
      </PresetFormProvider>
    </div>
  );
};
