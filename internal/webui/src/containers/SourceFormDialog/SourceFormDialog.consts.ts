import { Source } from "types/Source/Source";
import { toArrayFromRecord } from "utils/toArrayFromRecord";
import { toRecordFromArray } from "utils/toRecordFromArray";
import { SourceFormValues } from "./components/SourceForm/SourceForm.types";

export const getSourceInitialValues = (data?: Source): SourceFormValues => ({
  Name: data?.Name ?? "",
  Group: data?.Group ?? "default",
  ConnStr: data?.ConnStr ?? "",
  Kind: data?.Kind ?? "postgres",
  IsEnabled: data?.IsEnabled ?? true,
  IsSuperuser: data?.IsSuperuser ?? false,
  Metrics: toArrayFromRecord(data?.Metrics),
  MetricsStandby: toArrayFromRecord(data?.MetricsStandby),
  PresetMetrics: data?.PresetMetrics ?? "basic",
  PresetMetricsStandby: data?.PresetMetricsStandby ?? "",
  OnlyIfMaster: data?.OnlyIfMaster ?? false,
  CustomTags: toArrayFromRecord(data?.CustomTags),
  IncludePattern: data?.IncludePattern ?? "",
  ExcludePattern: data?.ExcludePattern ?? "",
});

export const createSourceRequest = (values: SourceFormValues): Source => ({
  Name: values.Name,
  Group: values.Group,
  ConnStr: values.ConnStr,
  Kind: values.Kind,
  IsEnabled: values.IsEnabled,
  IsSuperuser: values.IsSuperuser,
  Metrics: toRecordFromArray(values.Metrics),
  MetricsStandby: toRecordFromArray(values.MetricsStandby),
  PresetMetrics: values.PresetMetrics ?? "",
  PresetMetricsStandby: values.PresetMetricsStandby ?? "",
  OnlyIfMaster: values.OnlyIfMaster,
  CustomTags: toRecordFromArray(values.CustomTags),
  IncludePattern: values.IncludePattern ?? "",
  ExcludePattern: values.ExcludePattern ?? "",
  HostConfig: {},
});
