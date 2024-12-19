import CheckIcon from "@mui/icons-material/Check";
import { GridColDef } from "@mui/x-data-grid";
import { MetricGridRow } from "./MetricsGrid.types";
import { MetricsGridActions } from "./components/MetricsGridActions/MetricsGridActions";
import { SqlPopUp } from "./components/SqlPopUp/SqlPopUp";

const getIcon = (value: boolean) => {
  if (value) {
    return <CheckIcon color="success" />;
  }
  return null;
};

export const useMetricsGridColumns = (): GridColDef<MetricGridRow>[] => ([
  {
    field: "Key",
    headerName: "Name",
    width: 150,
    align: "left",
    headerAlign: "left",
  },
  {
    field: "Description",
    headerName: "Description",
    width: 200,
    align: "center",
    headerAlign: "center",
    valueGetter: ({ row }) => row.Metric.Description,
  },
  {
    field: "InitSQL",
    headerName: "InitSQL",
    width: 200,
    align: "center",
    headerAlign: "center",
    valueGetter: ({ row }) => row.Metric.InitSQL,
  },
  {
    field: "NodeStatus",
    headerName: "Node status",
    width: 200,
    align: "center",
    headerAlign: "center",
    valueGetter: ({ row }) => row.Metric.NodeStatus,
  },
  {
    field: "Gauges",
    headerName: "Gauges",
    width: 200,
    align: "center",
    headerAlign: "center",
    valueGetter: ({ row }) => row.Metric.Gauges && row.Metric.Gauges.toString(),
  },
  {
    field: "IsInstanceLevel",
    headerName: "Instance level?",
    width: 120,
    align: "center",
    headerAlign: "center",
    renderCell: ({ row }) => getIcon(row.Metric.IsInstanceLevel),
  },
  {
    field: "StorageName",
    headerName: "Storage name",
    width: 200,
    align: "center",
    headerAlign: "center",
    valueGetter: ({ row }) => row.Metric.StorageName,
  },
  {
    field: "SQLs",
    headerName: "SQLs",
    width: 120,
    align: "center",
    headerAlign: "center",
    renderCell: ({ row }) => <SqlPopUp SQLs={row.Metric.SQLs} />,
  },
  {
    field: "Actions",
    headerName: "Actions",
    headerAlign: "center",
    renderCell: ({ row }) => <MetricsGridActions metric={row} />
  },
]);
