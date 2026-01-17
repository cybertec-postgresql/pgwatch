import { MetricGridRow } from "pages/MetricsPage/components/MetricsGrid/MetricsGrid.types";
import { MetricRequestBody } from "types/Metric/MetricRequestBody";
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
    SQLs: data?.Metric.SQLs
      ? Object.keys(data.Metric.SQLs).map((version) => ({
        Version: Number(version),
        SQL: data.Metric.SQLs[Number(version)]
      }))
      : [],
  };
};

export const createMetricRequest = (values: MetricFormValues): MetricRequestBody => {
  const sqls: Record<number, string> = {};

  if (values.SQLs && values.SQLs.length > 0) {
    values.SQLs.forEach(({ Version, SQL }) => {
      sqls[Version] = SQL;
    });
  }

  return {
    Name: values.Name,
    Data: {
      StorageName: values.StorageName,
      NodeStatus: values.NodeStatus,
      Description: values.Description,
      Gauges: values.Gauges?.split("\n"),
      InitSQL: values.InitSQL,
      IsInstanceLevel: values.IsInstanceLevel,
      SQLs: sqls,
    },
  };
};
