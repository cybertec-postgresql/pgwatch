import { GridColDef } from "@mui/x-data-grid";

export const useCustomTagsPopUpColumns = (): GridColDef[] => ([
  {
    field: "name",
    headerName: "Name",
    flex: 1,
  },
  {
    field: "value",
    headerName: "Value",
    align: "center",
    headerAlign: "center",
    flex: 1,
  },
]);
