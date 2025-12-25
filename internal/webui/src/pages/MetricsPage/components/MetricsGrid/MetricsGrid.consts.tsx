import CheckIcon from "@mui/icons-material/Check";
import { GridColDef } from "@mui/x-data-grid";
import { MetricGridRow } from "./MetricsGrid.types";
import { ListPopUp } from "./components/ListPopUp/ListPopUp";
import { MetricsGridActions } from "./components/MetricsGridActions/MetricsGridActions";
import { SqlPopUp } from "./components/SqlPopUp/SqlPopUp";
import { TextPopUp } from "./components/TextPopUp/TextPopUp";

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
    flex: 1.5,
    minWidth: 150,
    align: "left",
    headerAlign: "left",
  },
  {
    field: "Description",
    headerName: "Description",
    flex: 0.5,
    minWidth: 100,
    align: "center",
    headerAlign: "center",
    renderCell: ({ row }) => (
      <TextPopUp
        title={`Description: ${row.Key}`}
        content={row.Metric.Description}
        type="description"
      />
    ),
  },
  {
    field: "InitSQL",
    headerName: "InitSQL",
    flex: 0.5,
    minWidth: 80,
    align: "center",
    headerAlign: "center",
    renderCell: ({ row }) => (
      <TextPopUp
        title={`InitSQL: ${row.Key}`}
        content={row.Metric.InitSQL}
        type="sql"
      />
    ),
  },
  {
    field: "NodeStatus",
    headerName: "Node status",
    flex: 1,
    minWidth: 120,
    align: "center",
    headerAlign: "center",
    valueGetter: (value, row) => row.Metric.NodeStatus,
  },
  {
    field: "Gauges",
    headerName: "Gauges",
    flex: 0.5,
    minWidth: 100,
    align: "center",
    headerAlign: "center",
    renderCell: ({ row }) => (
      <ListPopUp
        title={`Gauges: ${row.Key}`}
        items={row.Metric.Gauges}
      />
    ),
  },
  {
    field: "IsInstanceLevel",
    headerName: "Instance level?",
    flex: 0.5,
    minWidth: 100,
    align: "center",
    headerAlign: "center",
    renderCell: ({ row }) => getIcon(row.Metric.IsInstanceLevel),
  },
  {
    field: "StorageName",
    headerName: "Storage name",
    flex: 1,
    minWidth: 120,
    align: "center",
    headerAlign: "center",
    valueGetter: (value, row) => row.Metric.StorageName,
  },
  {
    field: "SQLs",
    headerName: "SQLs",
    flex: 0.5,
    minWidth: 80,
    align: "center",
    headerAlign: "center",
    renderCell: ({ row }) => <SqlPopUp SQLs={row.Metric.SQLs} />,
  },
  {
    field: "Actions",
    headerName: "Actions",
    flex: 0.5,
    minWidth: 100,
    headerAlign: "center",
    renderCell: ({ row }) => <MetricsGridActions metric={row} />
  },
]);
