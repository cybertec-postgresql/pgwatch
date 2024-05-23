import { useState } from "react";

import { Box, Typography } from "@mui/material";
import { DataGrid } from "@mui/x-data-grid";

import { ErrorComponent } from "layout/common/ErrorComponent";
import { databasesColumns } from "layout/common/Grid/GridColumns";
import { GridToolbarComponent } from "layout/common/Grid/GridToolbarComponent";
import { LoadingComponent } from "layout/common/LoadingComponent";

import { useDbs, useDeleteDb } from "queries/Source";
import { Db } from "queries/types/DbTypes";

import { ModalComponent } from "./ModalComponent";

export const DbsTable = () => {
  const [modalOpen, setModalOpen] = useState(false);
  const [editData, setEditData] = useState<Db>();
  const [action, setAction] = useState<"NEW" | "EDIT" | "DUPLICATE">("NEW");

  const { status, data, error } = useDbs();

  const deleteRecord = useDeleteDb();

  const handleModalOpen = (state: "NEW" | "EDIT" | "DUPLICATE") => {
    setAction(state);
    setModalOpen(true);
  };

  const columns = databasesColumns(
    {
      setEditData,
      handleModalOpen,
      deleteRecord
    }
  );

  if (status === "loading") {
    return (
      <LoadingComponent />
    );
  };

  if (status === "error") {
    const err = error as Error;
    return (
      <ErrorComponent errorMessage={err.message} />
    );
  };

  return (
    <Box display="flex" flexDirection="column" gap={1} height="100%">
      <Typography variant="h5">
        Databases under monitoring
      </Typography>
      <DataGrid
        columns={columns}
        rows={data}
        getRowId={(row) => row.DBUniqueName}
        rowsPerPageOptions={[]}
        components={{ Toolbar: () => <GridToolbarComponent handleModalOpen={handleModalOpen} setEditData={setEditData} /> }}
        disableColumnMenu
        initialState={{
          sorting: {
            sortModel: [{ field: "DBUniqueName", sort: "asc" }]
          }
        }}
      />
      <ModalComponent open={modalOpen} setOpen={setModalOpen} recordData={editData} action={action} />
    </Box>
  );
};
