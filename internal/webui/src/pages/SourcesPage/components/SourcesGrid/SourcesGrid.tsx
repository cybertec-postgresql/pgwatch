import { DataGrid } from "@mui/x-data-grid";
import { Error } from "components/Error/Error";
import { Loading } from "components/Loading/Loading";
import { SourceFormDialog } from "containers/SourceFormDialog/SourceFormDialog";
import { SourceFormProvider } from "contexts/SourceForm/SourceForm.provider";
import { useGridState } from 'hooks/useGridState';
import { usePageStyles } from "styles/page";
import { useSources } from "queries/Source";
import { useSourcesGridColumns } from "./SourcesGrid.consts";
import { SourcesGridToolbar } from "./components/SourcesGridToolbar";

export const SourcesGrid = () => {
  const { classes } = usePageStyles();

  const { data, isLoading, isError, error } = useSources();

  const columns = useSourcesGridColumns();
  const { 
    columnVisibility, 
    columnsWithSizing, 
    onColumnVisibilityChange, 
    onColumnResize
  } = useGridState('SOURCES_GRID', columns, {
    Kind: false,
    IncludePattern: false,
    ExcludePattern: false,
    PresetMetrics: false,
    PresetMetricsStandby: false,
    CustomTags: false,
    HostConfig: false
  });

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
      <SourceFormProvider>
        <DataGrid
          getRowId={(row) => row.Name}
          columns={columnsWithSizing}
          rows={data ?? []}
          pageSizeOptions={[]}
          slots={{ 
            toolbar: () => <SourcesGridToolbar />
          }}
          columnVisibilityModel={columnVisibility}
          onColumnVisibilityModelChange={onColumnVisibilityChange}
          onColumnResize={onColumnResize}
        />
        <SourceFormDialog />
      </SourceFormProvider>
    </div>
  );
};
