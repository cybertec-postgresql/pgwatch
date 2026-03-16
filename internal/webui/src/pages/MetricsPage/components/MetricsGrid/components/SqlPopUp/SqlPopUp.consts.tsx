import { GridColDef } from "@mui/x-data-grid";
import { Light as SyntaxHighlighter } from "react-syntax-highlighter";
import sql from "react-syntax-highlighter/dist/esm/languages/hljs/sql";
import { atomOneLight } from "react-syntax-highlighter/dist/esm/styles/hljs";

SyntaxHighlighter.registerLanguage("sql", sql);

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
       <SyntaxHighlighter
        language="sql"
        style={atomOneLight}
        wrapLongLines
        customStyle={{
          fontSize: "0.75rem",
          borderRadius: "4px",
          width: "100%",
          margin: 0,
        }}
      >
        {value}
      </SyntaxHighlighter>
    ),
  },
];
