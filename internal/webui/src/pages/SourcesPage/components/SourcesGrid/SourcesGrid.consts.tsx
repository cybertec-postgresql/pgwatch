import CheckIcon from "@mui/icons-material/Check";
import { GridColDef } from "@mui/x-data-grid";
import { MetricPopUp } from "components/MetricPopUp/MetricPopUp";
import { Source } from "types/Source/Source";
import { CustomTagsPopUp } from "./components/CustomTagsPopUp/CustomTagsPopUp";
import { EnabledSourceSwitch } from "./components/EnabledSourceSwitch";
import { HostConfigPopUp } from "./components/HostConfigPopUp/HostConfigPopUp";
import { SourcesGridActions } from "./components/SourcesGridActions";

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
    minWidth: 300,
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
    field: "Metrics Standby",
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
    headerName: "Metrics preset",
    width: 150,
    align: "center",
    headerAlign: "center",
    hide: true,
  },
  {
    field: "PresetMetricsStandby",
    headerName: "Metrics standby preset",
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
    renderCell: ({ row }) => <EnabledSourceSwitch source={row} />,
  },
  {
    field: "CustomTags",
    headerName: "Custom tags",
    width: 120,
    align: "center",
    headerAlign: "center",
    renderCell: ({ row }) => <CustomTagsPopUp CustomTags={row.CustomTags} />,
    hide: true,
  },
  {
    field: "HostConfig",
    headerName: "Host config",
    width: 120,
    align: "center",
    headerAlign: "center",
    renderCell: ({row}) => <HostConfigPopUp source={row} />,
    hide: true,
  },
  {
    field: "OnlyIfMaster",
    headerName: "Primary mode only",
    width: 120,
    align: "center",
    headerAlign: "center",
    renderCell: ({ value }) => getIcon(value),
  },
  {
    field: "Actions",
    headerName: "Actions",
    headerAlign: "center",
    width: 150,
    renderCell: ({ row }) => <SourcesGridActions source={row} />
  }
]);
