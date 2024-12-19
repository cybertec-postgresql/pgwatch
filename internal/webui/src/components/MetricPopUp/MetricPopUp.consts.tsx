import { GridColDef } from "@mui/x-data-grid";

export const useMetricPopUpColumns = (): GridColDef[] => ([
  {
    field: "name",
    headerName: "Name",
    flex: 1,
  },
  {
    field: "interval",
    headerName: "Update interval",
    align: "center",
    headerAlign: "center",
    flex: 1,
  },
]);
