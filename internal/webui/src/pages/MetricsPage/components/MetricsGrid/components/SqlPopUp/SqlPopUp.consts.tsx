import { GridColDef } from "@mui/x-data-grid";
import { Light as SyntaxHighlighter } from "react-syntax-highlighter";
import sql from "react-syntax-highlighter/dist/esm/languages/hljs/sql";
import { atomOneDark } from "react-syntax-highlighter/dist/esm/styles/hljs";

SyntaxHighlighter.registerLanguage("sql", sql);

export const useSqlPopUpColumns = (): GridColDef[] => [
  {
    field: "version",
    headerName: "Version",
    width: 80,
  },
  {
    field: "sql",
    headerName: "SQL",
    headerAlign: "left",
    flex: 1,
    renderCell: (params) => (
      <SyntaxHighlighter
        language="sql"
        style={atomOneDark}
        wrapLongLines={true}
        customStyle={{
          margin: 0,
          fontSize: "12px",
          borderRadius: "4px",
          width: "100%",
        }}
      >
        {params.value}
      </SyntaxHighlighter>
    ),
  },
];
