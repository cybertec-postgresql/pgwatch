import { Box, Popover, Typography } from '@mui/material';
import { useState, useEffect } from 'react';

type Params = {
    value: string
}

export const PasswordTypography = ({ value }: Params) => {
    const [anchorEl, setAnchorEl] = useState<HTMLSpanElement | null>(null);

    const open = Boolean(anchorEl);

    return (
        <Box sx={{ maxWidth: "150px" }}>
            <Typography sx={{ maxWidth: "150px", ":hover": { textDecoration: "underline", fontWeight: "bold" } }} onClick={(e) => setAnchorEl(e.currentTarget)}>{value.replaceAll(/./g, "*").substring(0, 15)}</Typography>
            <Popover
                id="popover"
                open={open}
                anchorEl={anchorEl}
                onClose={() => setAnchorEl(null)}
                anchorOrigin={{
                    vertical: "bottom",
                    horizontal: "center",
                }}
                transformOrigin={{
                    vertical: "top",
                    horizontal: "center",
                }}
            >
                <Typography sx={{ p: 2, maxWidth: "fit-content", overflowX: "auto" }}>{value}</Typography>
            </Popover>
        </Box>
    );
}
