import { PresetFormSteps } from "./PresetForm.consts";

export type PresetFormStep = keyof typeof PresetFormSteps;

type PresetFormGeneral = {
  Name: string;
  Description?: string;
};

type PresetFormMetrics = {
  Metrics: {
    Name: string;
    Interval: number;
  }[];
};

export type PresetFormValues = PresetFormGeneral & PresetFormMetrics;
