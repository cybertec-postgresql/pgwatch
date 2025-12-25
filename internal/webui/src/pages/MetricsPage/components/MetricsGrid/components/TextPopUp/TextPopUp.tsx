import { useState } from "react";
import DataObjectIcon from '@mui/icons-material/DataObject';
import DescriptionOutlinedIcon from '@mui/icons-material/DescriptionOutlined';
import RemoveCircleOutlineIcon from '@mui/icons-material/RemoveCircleOutline';
import { Box, Dialog, DialogContent, DialogTitle, IconButton, Tooltip, Typography } from "@mui/material";
import ReactMarkdown from 'react-markdown';

type Props = {
  title: string;
  content: string;
  type?: "description" | "sql";
};

export const TextPopUp = ({ title, content, type = "description" }: Props) => {
  const [open, setOpen] = useState(false);

  const handleOpen = () => setOpen(true);

  const handleClose = () => setOpen(false);

  const hasContent = content && content.trim() !== "";

  if (!hasContent) {
    return (
      <Tooltip title="None">
        <span>
          <RemoveCircleOutlineIcon fontSize="small" color="disabled" />
        </span>
      </Tooltip>
    );
  }

  const Icon = type === "sql" ? DataObjectIcon : DescriptionOutlinedIcon;
  const tooltipText = type === "sql" ? "View InitSQL" : "View Description";

  return (
    <>
      <IconButton onClick={handleOpen} size="small">
        <Tooltip title={tooltipText}>
          <span>
            <Icon fontSize="small" color="primary" />
          </span>
        </Tooltip>
      </IconButton>
      <Dialog
        open={open}
        onClose={handleClose}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle>{title}</DialogTitle>
        <DialogContent>
          <Box
            sx={{
              backgroundColor: type === "sql" ? "#f5f5f5" : "transparent",
              padding: 2,
              borderRadius: 1,
              maxHeight: 500,
              overflow: "auto",
            }}
          >
            {type === "sql" ? (
              <Typography
                component="pre"
                sx={{
                  margin: 0,
                  fontFamily: "monospace",
                  fontSize: "0.875rem",
                  whiteSpace: "pre-wrap",
                  wordBreak: "break-word",
                }}
              >
                {content}
              </Typography>
            ) : (
              <Box sx={{ 
                '& p': { marginTop: 0, marginBottom: 1 },
                '& h1, & h2, & h3, & h4, & h5, & h6': { marginTop: 2, marginBottom: 1 },
                '& ul, & ol': { paddingLeft: 3 },
                '& code': { 
                  backgroundColor: '#f5f5f5', 
                  padding: '2px 4px', 
                  borderRadius: 1,
                  fontFamily: 'monospace',
                },
                '& pre': { 
                  backgroundColor: '#f5f5f5', 
                  padding: 2, 
                  borderRadius: 1,
                  overflow: 'auto',
                },
              }}
              >
                <ReactMarkdown>{content}</ReactMarkdown>
              </Box>
            )}
          </Box>
        </DialogContent>
      </Dialog>
    </>
  );
};
