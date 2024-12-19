import { MetricFormSteps } from "./MetricForm.consts";

export type MetricFormStep = keyof typeof MetricFormSteps;

type MetricFormGeneral = {
  Name: string;
  StorageName?: string | null;
  NodeStatus?: string | null;
  Description?: string | null;
};

type MetricFormSettings = {
  Gauges?: string | null;
  InitSQL?: string | null;
  IsInstanceLevel: boolean;
};

type MetricFormSQL = {
  SQLs: string;
};

export type MetricFormValues = MetricFormGeneral & MetricFormSettings & MetricFormSQL;
