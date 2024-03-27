import { GridColDef } from "@mui/x-data-grid";

export const useSqlPopUpColumns = (): GridColDef[] => [
  {
    field: "version",
    headerName: "Version",
  },
  {
    field: "sql",
    headerName: "SQL",
    align: "center",
    headerAlign: "center",
    width: 600,
  },
];
