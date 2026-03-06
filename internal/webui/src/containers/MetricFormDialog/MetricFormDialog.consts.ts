import { MetricGridRow } from "pages/MetricsPage/components/MetricsGrid/MetricsGrid.types";
import { MetricRequestBody } from "types/Metric/MetricRequestBody";
import yaml from "yaml";
import { MetricFormValues } from "./components/MetricForm/MetricForm.types";

export const convertGauges = (data: string[] | null) => data && data.toString().replace(/,/g, "\n");

export const getMetricInitialValues = (data?: MetricGridRow): MetricFormValues => {
  return {
    Name: data?.Key ?? "",
    StorageName: data?.Metric.StorageName ?? "",
    NodeStatus: data?.Metric.NodeStatus ?? "",
    Description: data?.Metric.Description ?? "",
    Gauges: convertGauges(data?.Metric.Gauges ?? [""]),
    InitSQL: data?.Metric.InitSQL ?? "",
    IsInstanceLevel: data?.Metric.IsInstanceLevel ?? false,
    SQLs: (data?.Metric.SQLs) ?? "",
  };
};

export const createMetricRequest = (values: MetricFormValues): MetricRequestBody => {
  const formattedSQLs = values.SQLs.reduce((acc, current) => {
    // Key = Version, Value = SQL
    acc[current.Version] = current.SQL; 
    return acc;
  }, {} as Record<string, string>);

  return {
    Name: values.Name,
    Data: {
      StorageName: values.StorageName,
      NodeStatus: values.NodeStatus,
      Description: values.Description,
      Gauges: values.Gauges?.split("\n"),
      InitSQL: values.InitSQL,
      IsInstanceLevel: values.IsInstanceLevel,
      SQLs: formattedSQLs,
    },
  };
};
