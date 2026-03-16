import { useMemo, useState } from "react";
import TableViewIcon from '@mui/icons-material/TableView';
import { Box, Chip, Dialog, DialogContent, DialogTitle, IconButton, Tooltip } from "@mui/material";
import { DataGrid } from "@mui/x-data-grid";
import { useSqlPopUpColumns } from "./SqlPopUp.consts";

type SQLRows = {
  version: number;
  sql: string;
};

type Props = {
  SQLs: Record<number, string>;
};

export const SqlPopUp = ({ SQLs }: Props) => {
  const [open, setOpen] = useState(false);

  const rows: SQLRows[] = useMemo(() => {
    return Object.keys(SQLs).map((key) => ({
      version: Number(key),
      sql: SQLs[Number(key)],
    }));
  }, [SQLs]);

  const columns = useSqlPopUpColumns();

  const handleOpen = () => setOpen(true);

  const handleClose = () => setOpen(false);

  return rows.length !== 0 ? (
    <>
      <IconButton onClick={handleOpen} size="small">
        <Tooltip title={`View SQLs (${rows.length})`}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
            <TableViewIcon fontSize="small" color="primary" />
            <Chip label={rows.length} size="small" variant="outlined" sx={{ height: 20, fontSize: "0.75rem" }} />
          </Box>
        </Tooltip>
      </IconButton>
      <Dialog
        open={open}
        onClose={handleClose}
        maxWidth="xl"        // SQL lines are long, needs full width
        fullWidth            // stretches to maxWidth instead of hugging content
      >
        <DialogTitle>SQLs</DialogTitle>
        <DialogContent sx={{maxHeight: 700 }}>
          <DataGrid
            getRowId={(row) => row.version}
            columns={columns}
            rows={rows}
            pageSizeOptions={[]}
            getRowHeight={() => "auto"}
            autoHeight
            disableColumnMenu
            sx={{
              // let the pre block breathe inside auto-height rows
              "& .MuiDataGrid-cell": {
                alignItems: "flex-start",
                py: 1,
              },
            }}
          />
        </DialogContent>
      </Dialog>
    </>
  ) : null;
};
