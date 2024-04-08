import { useMemo, useState } from "react";
import { DataGrid } from "@mui/x-data-grid";
import { MetricFormDialog } from "containers/MetricFormDialog/MetricFormDialog";
import { MetricFormProvider } from "contexts/MetricForm/MetricForm.provider";
import { ErrorComponent } from "layout/common/ErrorComponent";
import { LoadingComponent } from "layout/common/LoadingComponent";
import { useMetrics } from "queries/Metric";
import { useMetricsGridColumns } from "./MetricsGrid.consts";
import { Root } from "./MetricsGrid.styles";
import { MetricGridRow } from "./MetricsGrid.types";
import { MetricsGridToolbar } from "./components/MetricsGridToolbar/MetricsGridToolbar";

export const MetricsGrid = () => {
  const [formDialogOpen, setFormDialogOpen] = useState(false);

  const { status, data, error } = useMetrics();

  const rows: MetricGridRow[] | [] = useMemo(() => {
    if (data) {
      return Object.keys(data).map((key) => {
        const metric = data[key];
        return {
          Key: key,
          Metric: metric,
        };
      });
    }
    return [];
  }, [data]);

  const columns = useMetricsGridColumns();

  const handleFormDialogOpen = () => setFormDialogOpen(true);

  const handleFormDialogClose = () => setFormDialogOpen(false);

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
    <Root>
      <MetricFormProvider
        open={formDialogOpen}
        handleOpen={handleFormDialogOpen}
        handleClose={handleFormDialogClose}
      >
        <DataGrid
          getRowId={(row) => row.Key}
          columns={columns}
          rows={rows}
          rowsPerPageOptions={[]}
          components={{ Toolbar: () => <MetricsGridToolbar /> }}
          disableColumnMenu
        />
        <MetricFormDialog />
      </MetricFormProvider>
    </Root >
  );
};
