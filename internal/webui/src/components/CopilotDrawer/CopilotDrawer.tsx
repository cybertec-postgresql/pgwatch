import React from 'react';
import { Box, Drawer, Typography, IconButton, Divider, TextField, Stack, Avatar } from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import SmartToyIcon from '@mui/icons-material/SmartToy';

//Define the Props for TypeScript
interface CopilotDrawerProps {
    open: boolean;
    onClose: () => void;
}

export const CopilotDrawer: React.FC<CopilotDrawerProps> = ({ open, onClose }) => {
    return (
        <Drawer
            anchor="right"
            open={open}
            onClose={onClose}
            variant="temporary"
            PaperProps={{
                sx: { width: { xs: "100%", sm: 350 }, display: 'flex', flexDirection: 'column' },
            }}
        >
            <Box sx={{ p: 2, display: 'flex', alignItems: 'center', gap: 2, background: 'linear-gradient(45deg, #2196F3 30%, #21CBF3 90%)' }}>
                <Avatar sx={{ bgcolor: 'white' }}>
                    <SmartToyIcon sx={{ color: '#2196F3' }} />
                </Avatar>
                <Typography variant="h6" sx={{ flexGrow: 1 }}>
                    DB Copilot
                </Typography>
                <IconButton onClick={onClose} sx={{ color: 'white' }}>
                    <CloseIcon />
                </IconButton>
            </Box>
            <Divider />
            <Box sx={{ p: 2, bgcolor: 'white', borderRadius: 2, boxShadow: 1 }}>
                <Stack spacing={2}>
                    <Typography variant="subtitle2" color="primary">AI Suggestion</Typography>
                    <Typography variant="body2">
                        Hello! I'm ready to help you analyze your pgwatch metrics. Try asking about CPU spikes or slow queries!.
                    </Typography>
                </Stack>
            </Box>
            <Divider />
            {/*Input area*/}
            <Box sx={{ p: 2, bgcolor: 'white' }}>
                <TextField
                    fullWidth
                    placeholder="Ask something..."
                    variant="outlined"
                    size="small"
                    onKeyPress={(e) => {
                        if (e.key === 'Enter') {
                            console.log("User asked: ", (e.target as HTMLInputElement).value);
                        }
                    }}
                />
            </Box>
        </Drawer>
    )
}