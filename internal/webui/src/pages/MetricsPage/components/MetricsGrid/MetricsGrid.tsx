import { useMemo } from "react";
import { DataGrid } from "@mui/x-data-grid";
import { Error } from "components/Error/Error";
import { Loading } from "components/Loading/Loading";
import { MetricFormDialog } from "containers/MetricFormDialog/MetricFormDialog";
import { MetricFormProvider } from "contexts/MetricForm/MetricForm.provider";
import { usePageStyles } from "styles/page";
import { useMetrics } from "queries/Metric";
import { useMetricsGridColumns } from "./MetricsGrid.consts";
import { MetricGridRow } from "./MetricsGrid.types";
import { MetricsGridToolbar } from "./components/MetricsGridToolbar/MetricsGridToolbar";

export const MetricsGrid = () => {
  const { data, isLoading, isError, error } = useMetrics();

  const { classes } = usePageStyles();

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

  if (isLoading) {
    return (
      <Loading />
    );
  };

  if (isError) {
    const err = error as Error;
    return (
      <Error message={err.message} />
    );
  };

  return (
    <div className={classes.page}>
      <MetricFormProvider>
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
    </div >
  );
};
