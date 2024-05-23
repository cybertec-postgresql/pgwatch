import { useState } from "react";
import { DataGrid } from "@mui/x-data-grid";
import { usePageStyles } from "styles/page";
import { ErrorComponent } from "layout/common/ErrorComponent";
import { LoadingComponent } from "layout/common/LoadingComponent";
import { useSources } from "queries/Source";
import { useSourcesGridColumns } from "./SourcesGrid.consts";

export const SourcesGrid = () => {
  const [formDialogOpen, setFormDialogOpen] = useState(false);

  const { classes } = usePageStyles();

  const { status, data, error } = useSources();

  const columns = useSourcesGridColumns();

  if (status === 'loading') {
    return (
      <LoadingComponent />
    );
  }

  if (status === 'error') {
    const err = error as Error;
    return (
      <ErrorComponent errorMessage={err.message} />
    );
  }

  return (
    <div className={classes.page}>
      <DataGrid
        getRowId={(row) => row.DBUniqueName}
        columns={columns}
        rows={data}
        rowsPerPageOptions={[]}
        //components={{ Toolbar: () => <></> }} TODO
        disableColumnMenu
      />
    </div>
  );
};
