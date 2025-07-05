import { useMemo, useState } from "react";
import TableViewIcon from "@mui/icons-material/TableView";
import { Dialog, DialogContent, IconButton } from "@mui/material";
import { DataGrid } from "@mui/x-data-grid";
import { useCustomTagsPopUpColumns } from "./CustomTagsPopUp.consts";


type CustomTagsRows = {
  name: string;
  value: string;
};

type Props = {
  CustomTags?: Record<string, string> | null;
};

export const CustomTagsPopUp = ({ CustomTags }: Props) => {
  const [open, setOpen] = useState(false);

  const rows: CustomTagsRows[] = useMemo(() => {
    if (CustomTags) {
      return Object.keys(CustomTags).map((key) => ({
        name: key,
        value: CustomTags[key],
      }));
    }
    return [];
  }, [CustomTags]);

  const columns = useCustomTagsPopUpColumns();

  const handleOpen = () => setOpen(true);

  const handleClose = () => setOpen(false);

  return rows.length !== 0 ? (
    <>
      <IconButton title="View custom tags table" onClick={handleOpen}>
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
            pageSizeOptions={[]}
            autoHeight
          />
        </DialogContent>
      </Dialog>
    </>
  ) : null;
};
