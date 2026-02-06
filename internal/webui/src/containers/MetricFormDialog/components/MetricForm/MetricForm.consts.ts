import * as Yup from "yup";

export enum MetricFormSteps {
  General = "General",
  Settings = "Settings",
  SQLs = "SQLs",
};

export const metricFormValuesValidationSchema = Yup.object({
  Name: Yup.string().trim().required("Name is required"),
  StorageName: Yup.string().optional().nullable(),
  NodeStatus: Yup.string().optional().nullable(),
  Description: Yup.string().optional().nullable(),
  Gauges: Yup.string().optional().nullable(),
  InitSQL: Yup.string().optional().nullable(),
  IsInstanceLevel: Yup.bool().required(),
  SQLs: Yup.array().of(Yup.object().shape({Version: Yup.string().required("Version is required"),SQL: Yup.string().required("SQL value is required"),})),
});
