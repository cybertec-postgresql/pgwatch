export type Source = {
  DBUniqueName: string;
  DBUniqueNameOrig: string;
  Group: string;
  ConnStr: string;
  Metrics: Record<string, number> | null;
  MetricsStandby: Record<string, number> | null;
  Kind: string;
  IncludePattern: string;
  ExcludePattern: string;
  PresetMetrics: string;
  PresetMetricsStandby: string;
  IsSuperuser: boolean;
  IsEnabled: boolean;
  CustomTags: Record<string, string>;
  HostConfig: string;
  OnlyIfMaster: boolean;
};
