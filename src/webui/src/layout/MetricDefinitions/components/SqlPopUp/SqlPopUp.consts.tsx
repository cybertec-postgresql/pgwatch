import { GridColDef } from "@mui/x-data-grid";

export const columns: GridColDef[] = [
  {
    field: "version",
    headerName: "Version",
  },
  {
    field: "sql",
    headerName: "SQL",
    align: "center",
    headerAlign: "center",
    width: 300,
  },
];
