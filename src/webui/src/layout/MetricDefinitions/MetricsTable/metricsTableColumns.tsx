import CheckIcon from '@mui/icons-material/Check';
import CloseIcon from '@mui/icons-material/Close';
import { Box } from '@mui/material';
import { GridColDef, GridRenderCellParams } from "@mui/x-data-grid";
import moment from 'moment';

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
            return (Object.keys(params.value));
        },
    },
    {
        field: "m_comment",
        headerName: "Comment",
        width: 150,
        align: "center",
        headerAlign: "center"
    },
    {
        field: "m_is_active",
        headerName: "Enabled?",
        width: 120,
        renderCell: (params: GridRenderCellParams<boolean>) => params.value ? <CheckIcon /> : <CloseIcon />,
        align: "center",
        headerAlign: "center"
    },
    {
        field: "m_is_helper",
        headerName: "Helpers?",
        width: 120,
        renderCell: (params: GridRenderCellParams<boolean>) => params.value ? <CheckIcon /> : <CloseIcon />,
        align: "center",
        headerAlign: "center"
    },
    {
        field: "m_last_modified_on",
        headerName: "Last modified",
        width: 150,
        renderCell: (params) =>
            <Box sx={{ display: "flex", alignItems: "center", textAlign: "center" }}>
                {moment(params.value).format("MMMM Do YYYY HH:mm:ss")}
            </Box>,
        align: "center",
        headerAlign: "center"
    }
];
