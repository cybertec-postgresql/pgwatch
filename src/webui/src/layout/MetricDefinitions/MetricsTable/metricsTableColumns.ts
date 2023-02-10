import { GridColDef } from "@mui/x-data-grid";

export const columns: GridColDef[] = [
    {
        field: "m_name",
        headerName: "Metric name",
        width: 150,
        align: "center",
        headerAlign: "center"
    },
    {
        field: "m_versions",
        headerName: "PG versions",
        width: 150,
        align: "center",
        headerAlign: "center",
        valueGetter(params) {
            return(Object.keys(params.value));
        },
    },
    {
        field: "m_comment",
        headerName: "Comment",
        width: 150,
        align: "center",
        headerAlign: "center"
    }
];
