export type Source = {
  Name: string;
  Group: string;
  ConnStr: string;
  Metrics: Record<string, number> | null;
  MetricsStandby: Record<string, number> | null;
  Kind: string;
  IncludePattern: string;
  ExcludePattern: string;
  PresetMetrics: string;
  PresetMetricsStandby: string;
  IsEnabled: boolean;
  CustomTags: Record<string, string> | null;
  HostConfig: object;
  OnlyIfMaster: boolean;
};
