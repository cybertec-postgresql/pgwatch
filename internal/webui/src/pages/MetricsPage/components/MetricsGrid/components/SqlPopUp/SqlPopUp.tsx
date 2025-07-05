import { useMemo, useState } from "react";
import TableViewIcon from '@mui/icons-material/TableView';
import { Dialog, DialogContent, DialogTitle, IconButton, Tooltip } from "@mui/material";
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
      <IconButton onClick={handleOpen}>
        <Tooltip title="View SQLs table">
          <span>
            <TableViewIcon />
          </span>
        </Tooltip>
      </IconButton>
      <Dialog
        open={open}
        onClose={handleClose}
        maxWidth="md"
      >
        <DialogTitle>SQLs</DialogTitle>
        <DialogContent sx={{ width: 750, maxHeight: 600 }}>
          <DataGrid
            getRowId={(row) => row.version}
            columns={columns}
            rows={rows}
            pageSizeOptions={[]}
            getRowHeight={() => "auto"}
            autoHeight
          />
        </DialogContent>
      </Dialog>
    </>
  ) : null;
};
