import { SourceFormSteps } from "./SourceForm.consts";

export type SourceFormStep = keyof typeof SourceFormSteps;

type SourceFormGeneral = {
  Name: string;
  Group: string;
  ConnStr: string;
  Kind: string;
  IncludePattern?: string | null;
  ExcludePattern?: string | null;
  IsEnabled: boolean;
  IsSuperuser: boolean;
};

type SourceFormMetrics = {
  Metrics?: {
    Name: string;
    Value: number;
  }[] | null;
  MetricsStandby?: {
    Name: string;
    Value: number;
  }[] | null;
  PresetMetrics?: string | null;
  PresetMetricsStandby?: string | null;
  OnlyIfMaster: boolean;
};

type SourceFormTags = {
  CustomTags?: {
    Name: string;
    Value: string;
  }[] | null,
};

export type SourceFormValues =
  SourceFormGeneral &
  SourceFormMetrics &
  SourceFormTags;
