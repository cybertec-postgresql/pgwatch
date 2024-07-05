import { useState } from "react";
import { DataGrid } from "@mui/x-data-grid";
import { Error } from "components/Error/Error";
import { Loading } from "components/Loading/Loading";
import { SourceFormDialog } from "containers/SourceFormDialog/SourceFormDialog";
import { SourceFormProvider } from "contexts/SourceForm/SourceForm.provider";
import { usePageStyles } from "styles/page";
import { useSources } from "queries/Source";
import { useSourcesGridColumns } from "./SourcesGrid.consts";
import { SourcesGridToolbar } from "./components/SourcesGridToolbar";

export const SourcesGrid = () => {
  const [formDialogOpen, setFormDialogOpen] = useState(false);

  const { classes } = usePageStyles();

  const { data, isLoading, isError, error } = useSources();

  const columns = useSourcesGridColumns();

  const handleFormDialogOpen = () => setFormDialogOpen(true);

  const handleFormDialogClose = () => setFormDialogOpen(false);

  if (isLoading) {
    return (
      <Loading />
    );
  }

  if (isError) {
    const err = error as Error;
    return (
      <Error message={err.message} />
    );
  }

  return (
    <div className={classes.page}>
      <SourceFormProvider
        open={formDialogOpen}
        handleOpen={handleFormDialogOpen}
        handleClose={handleFormDialogClose}
      >
        <DataGrid
          getRowId={(row) => row.DBUniqueName}
          columns={columns}
          rows={data ?? []}
          rowsPerPageOptions={[]}
          components={{ Toolbar: () => <SourcesGridToolbar /> }}
          disableColumnMenu
        />
        <SourceFormDialog />
      </SourceFormProvider>
    </div>
  );
};
