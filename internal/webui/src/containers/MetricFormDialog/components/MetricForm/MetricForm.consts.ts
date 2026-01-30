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
  SQLs: Yup.array()
    .of(
      Yup.object().shape({
        Version: Yup.number()
          .required("Version is required")
          .positive("Version must be a positive number")
          .integer("Version must be an integer"),
        SQL: Yup.string()
          .trim()
          .required("SQL is required")
      })
    )
    .min(1, "At least one SQL version is required")
    .required("SQLs is required")
    .test(
      "unique-versions",
      "Duplicate version numbers are not allowed",
      (value) => {
        if (!value || value.length === 0) {
          return true;
        }
        const versions = value.map(item => item.Version);
        const uniqueVersions = new Set(versions);
        return versions.length === uniqueVersions.size;
      }
    ),
});
