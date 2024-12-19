import { GridColDef } from "@mui/x-data-grid";
import { MetricPopUp } from "../../../../components/MetricPopUp/MetricPopUp";
import { PresetGridRow } from "./PresetsGrid.types";
import { PresetsGridActions } from "./components/PresetsGridActions/PresetsGridActions";

export const usePresetsGridColumns = (): GridColDef<PresetGridRow>[] => ([
  {
    field: "Key",
    headerName: "Name",
    width: 200,
    align: "left",
    headerAlign: "left",
  },
  {
    field: "Description",
    headerName: "Description",
    flex: 1,
    align: "center",
    headerAlign: "center",
    valueGetter: ({ row }) => row.Preset.Description,
  },
  {
    field: "Metrics",
    headerName: "Metrics",
    width: 120,
    align: "center",
    headerAlign: "center",
    renderCell: ({ row }) => <MetricPopUp Metrics={row.Preset.Metrics} />
  },
  {
    field: "Actions",
    headerName: "Actions",
    headerAlign: "center",
    renderCell: ({ row }) => <PresetsGridActions preset={row} />
  },
]);
