import CheckIcon from '@mui/icons-material/Check';
import { GridColDef } from "@mui/x-data-grid";
import { MetricRow } from "./MetricDefinitions.types";
import { SqlPopUp } from "./components/SqlPopUp/SqlPopUp";

const getIcon = (value: boolean) => value && <CheckIcon color="success" />;

export const metricsColumns = (): GridColDef<MetricRow>[] => ([
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
    field: "Enabled",
    headerName: "Enabled?",
    width: 120,
    align: "center",
    headerAlign: "center",
    renderCell: ({ row }) => getIcon(row.Metric.Enabled),
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
  }
]);
