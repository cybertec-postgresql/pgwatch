import { Box } from "@mui/material";
import { GridColDef } from "@mui/x-data-grid";

export const useSqlPopUpColumns = (): GridColDef[] => [
  {
    field: "version",
    headerName: "Version",
    width: 80,
    align: "center",
    headerAlign: "center",
  },
  {
    field: "sql",
    headerName: "SQL",
    headerAlign: "left",
    flex: 1,
    renderCell: ({ value }) => (
      <Box
        component="pre"
        sx={{
          fontFamily: "'Fira Code', 'Cascadia Code', 'Consolas', monospace",
          fontSize: "0.75rem",
          lineHeight: 1.6,
          whiteSpace: "pre-wrap",
          wordBreak: "break-word",
          overflowX: "auto",
          m: 0,
          py: 1.5,
          px: 0,
          color: "text.primary",
        }}
      >
        {value}
      </Box>
    ),
  },
];
