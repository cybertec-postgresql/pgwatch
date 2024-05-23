import CheckIcon from "@mui/icons-material/Check";
import { GridColDef } from "@mui/x-data-grid";
import { MetricPopUp } from "components/MetricPopUp/MetricPopUp";
import { Source } from "types/Source/Source";
import { EnabledSourceSwitch } from "./components/EnabledSourceSwitch";

const getIcon = (value: boolean) => {
  if (value) {
    return <CheckIcon color="success" />;
  }
  return <></>;
};

export const useSourcesGridColumns = (): GridColDef<Source>[] => ([
  {
    field: "DBUniqueName",
    headerName: "Name",
  },
  {
    field: "Group",
    headerName: "Group",
    width: 150,
    align: "center",
    headerAlign: "center",
  },
  {
    field: "ConnStr",
    headerName: "Connection string",
    flex: 1,
    align: "center",
    headerAlign: "center",
  },
  {
    field: "Metrics",
    width: 120,
    align: "center",
    headerAlign: "center",
    renderCell: ({ row }) => <MetricPopUp Metrics={row.Metrics} />
  },
  {
    field: "MetricsStandby",
    width: 120,
    align: "center",
    headerAlign: "center",
    renderCell: ({ row }) => <MetricPopUp Metrics={row.MetricsStandby} />
  },
  {
    field: "Kind",
    headerName: "Type",
    align: "center",
    headerAlign: "center",
    hide: true,
  },
  {
    field: "IncludePattern",
    headerName: "Inclusion pattern",
    width: 150,
    align: "center",
    headerAlign: "center",
    hide: true,
  },
  {
    field: "ExcludePattern",
    headerName: "Exclusion pattern",
    width: 150,
    align: "center",
    headerAlign: "center",
    hide: true,
  },
  {
    field: "PresetMetrics",
    headerName: "Metrics",
    width: 150,
    align: "center",
    headerAlign: "center",
    hide: true,
  },
  {
    field: "PresetMetricsStandby",
    headerName: "Metrics standby",
    width: 150,
    align: "center",
    headerAlign: "center",
    hide: true,
  },
  {
    field: "IsSuperuser",
    headerName: "Superuser",
    width: 120,
    align: "center",
    headerAlign: "center",
    renderCell: ({ row }) => getIcon(row.IsSuperuser),
  },
  {
    field: "IsEnabled",
    headerName: "Enabled",
    width: 120,
    align: "center",
    headerAlign: "center",
    renderCell: ({ row }) => <EnabledSourceSwitch uniqueName={row.DBUniqueName} enabled={row.IsEnabled} />,
  },
  {
    field: "CustomTags",
    headerName: "Custom tags",
    width: 120,
    align: "center",
    headerAlign: "center",
    //renderCell: ({ row }) => <CustomTagsPopUp />, TODO
    hide: true,
  },
  {
    field: "HostConfig",
    headerName: "Host config",
    width: 150,
    align: "center",
    headerAlign: "center",
  },
  {
    field: "OnlyIfMaster",
    headerName: "Master mode only",
    width: 120,
    align: "center",
    headerAlign: "center",
    renderCell: ({ value }) => getIcon(value),
  }
]);
