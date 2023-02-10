import { Box, Typography } from "@mui/material";
import { DataGrid } from "@mui/x-data-grid";
import { columns } from "./metricsTableColumns";


export const MetricsTable = () => {

    const data = [
        {
            "m_name": "backends",
            "m_versions": {
                "9.0": "sql",
                "9.1": "sql",
                "10.0": "sql",
                "15.0": "sql"
            },
            "m_comment": "comment",
            "m_is_active": false,
            "m_is_helper": false,
            "m_last_modified_on": Date.now()
        },
        {
            "m_name": "wsl",
            "m_versions": {
                "9.3": "sql",
                "9.5": "sql",
                "11.2": "sql",
                "14.7": "sql"
            },
            "m_comment": "comment",
            "m_is_active": false,
            "m_is_helper": false,
            "m_last_modified_on": Date.now()
        }
    ];

    return (
        <Box display="flex" flexDirection="column" gap={1}>
            <Typography variant="h5">
                Metrics
            </Typography>
            <DataGrid
                getRowHeight={() => "auto"}
                getRowId={(row) => row.m_name}
                columns={columns}
                rows={data}
                autoHeight
                pageSize={5}
                disableColumnMenu
            />
        </Box>
    );
};
