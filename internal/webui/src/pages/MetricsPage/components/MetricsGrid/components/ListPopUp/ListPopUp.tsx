import { useState } from "react";
import ListAltIcon from '@mui/icons-material/ListAlt';
import RemoveCircleOutlineIcon from '@mui/icons-material/RemoveCircleOutline';
import { Box, Chip, Dialog, DialogContent, DialogTitle, IconButton, List, ListItem, ListItemText, Tooltip } from "@mui/material";

type Props = {
  title: string;
  items: string[] | null;
};

export const ListPopUp = ({ title, items }: Props) => {
  const [open, setOpen] = useState(false);

  const handleOpen = () => setOpen(true);

  const handleClose = () => setOpen(false);

  const hasItems = items && items.length > 0;

  if (!hasItems) {
    return (
      <Tooltip title="None">
        <span>
          <RemoveCircleOutlineIcon fontSize="small" color="disabled" />
        </span>
      </Tooltip>
    );
  }

  return (
    <>
      <IconButton onClick={handleOpen} size="small">
        <Tooltip title={`View ${title} (${items.length})`}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            <ListAltIcon fontSize="small" color="primary" />
            <Chip label={items.length} size="small" variant="outlined" sx={{ height: 20, fontSize: '0.75rem' }} />
          </Box>
        </Tooltip>
      </IconButton>
      <Dialog
        open={open}
        onClose={handleClose}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>{title}</DialogTitle>
        <DialogContent>
          <List dense sx={{ maxHeight: 400, overflow: 'auto' }}>
            {items.map((item, index) => (
              <ListItem key={index} divider>
                <ListItemText 
                  primary={item} 
                  primaryTypographyProps={{ 
                    fontFamily: 'monospace',
                    fontSize: '0.9rem'
                  }} 
                />
              </ListItem>
            ))}
          </List>
        </DialogContent>
      </Dialog>
    </>
  );
};
