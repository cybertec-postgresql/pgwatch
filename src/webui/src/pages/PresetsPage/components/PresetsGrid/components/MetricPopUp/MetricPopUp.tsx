import { useMemo, useState } from "react";
import TableViewIcon from "@mui/icons-material/TableView";
import { Dialog, DialogContent, IconButton } from "@mui/material";
import { DataGrid } from "@mui/x-data-grid";
import { useMetricPopUpColumns } from "./MetricPopUp.consts";

type MetricRows = {
  name: string;
  interval: number;
};

type Props = {
  Metrics: Record<string, number>;
};

export const MetricPopUp = ({ Metrics }: Props) => {
  const [open, setOpen] = useState(false);

  const rows: MetricRows[] = useMemo(() => {
    return Object.keys(Metrics).map((key) => ({
      name: key,
      interval: Metrics[key],
    }));
  }, [Metrics]);

  const columns = useMetricPopUpColumns();

  const handleOpen = () => setOpen(true);

  const handleClose = () => setOpen(false);

  return rows.length !== 0 ? (
    <>
      <IconButton title="View metrics table" onClick={handleOpen}>
        <TableViewIcon />
      </IconButton>
      <Dialog
        open={open}
        onClose={handleClose}
        maxWidth="md"
      >
        <DialogContent sx={{ width: 450, maxHeight: 500 }}>
          <DataGrid
            getRowId={(row) => row.name}
            columns={columns}
            rows={rows}
            rowsPerPageOptions={[]}
            autoHeight
            disableColumnMenu
          />
        </DialogContent>
      </Dialog>
    </>
  ) : null;
};
