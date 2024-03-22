import { useMemo, useState } from "react";
import TableViewIcon from '@mui/icons-material/TableView';
import { Dialog, DialogContent, DialogTitle, IconButton, Tooltip } from "@mui/material";
import { DataGrid, gridClasses } from "@mui/x-data-grid";
import { columns } from "./SqlPopUp.consts";

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

  const handleOpen = () => setOpen(true)

  const handleClose = () => setOpen(false)
  console.log(rows);

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
        <DialogContent sx={{ width: 600, maxHeight: 600 }}>
          <DataGrid
            getRowId={(row) => row.version}
            columns={columns}
            rows={rows}
            getRowHeight={() => "auto"}
            autoHeight
            disableColumnMenu
          />
        </DialogContent>
      </Dialog>
    </>
  ) : (<p>No SQLs provided for this metric</p>);
};
