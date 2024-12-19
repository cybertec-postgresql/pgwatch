import * as Yup from "yup";

export enum SourceFormSteps {
  General = "General",
  Metrics = "Metrics",
  Tags = "Tags",
};

const kindValues = ["postgres", "postgres-continuous-discovery", "pgbouncer", "pgpool", "patroni", "patroni-continuous-discovery", "patroni-namespace-discovery"];

export const KindOptions = kindValues.map((val) => ({ label: val }));

const metricsValidationSchema = Yup.object({
  Name: Yup.string().required("Choose metric or delete it"),
  Value: Yup.number().typeError("Please provide a number")
    .required("Interval is required")
    .min(10, "Min. interval is 10")
    .max(604800, "Max. interval is 604800"),
});

const tagsValidationSchema = Yup.object({
  Name: Yup.string().required("Name is required"),
  Value: Yup.string().required("Value is required"),
});

export const sourceFormValuesValidationSchema = Yup.object({
  Name: Yup.string().required("Unique name is required"),
  Group: Yup.string().required("Group is required"),
  ConnStr: Yup.string().required("Connection string is required"),
  Kind: Yup.string().required("Type is required"),
  IncludePattern: Yup.string().optional().nullable(),
  ExcludePattern: Yup.string().optional().nullable(),
  IsEnabled: Yup.boolean().required(),
  IsSuperuser: Yup.boolean().required(),
  Metrics: Yup.array().of(metricsValidationSchema).test((arr, context) => {
    if (!arr) {
      return;
    }
    const list = arr.map(val => val.Name);
    const set = [...new Set(arr.map(val => val.Name))];
    if (list.length === set.length) {
      return true;
    }
    const idx = list.findIndex((v, i) => v !== set[i]);
    return context.createError({ path: `${context.path}[${idx}].Name`, message: "Name should be unique", type: "unique" });
  }).optional().nullable(),
  MetricsStandby: Yup.array().of(metricsValidationSchema).test((arr, context) => {
    if (!arr) {
      return;
    }
    const list = arr.map(val => val.Name);
    const set = [...new Set(arr.map(val => val.Name))];
    if (list.length === set.length) {
      return true;
    }
    const idx = list.findIndex((v, i) => v !== set[i]);
    return context.createError({ path: `${context.path}[${idx}].Name`, message: "Name should be unique", type: "unique" });
  }).optional().nullable(),
  PresetMetrics: Yup.string().when("Metrics", {
    is: (metrics: any) => !metrics || metrics.length === 0,
    then: (schema) => schema.required("Please either choose preset or add custom metrics"),
  }).optional().nullable(),
  PresetMetricsStandby: Yup.string().optional().nullable(),
  OnlyIfMaster: Yup.boolean().required(),
  CustomTags: Yup.array().of(tagsValidationSchema).test((arr, context) => {
    if (!arr) {
      return;
    }
    const list = arr.map(val => val.Name);
    const set = [...new Set(arr.map(val => val.Name))];
    if (list.length === set.length) {
      return true;
    }
    const idx = list.findIndex((v, i) => v !== set[i]);
    return context.createError({ path: `${context.path}[${idx}].Name`, message: "Name should be unique", type: "unique" });
  }).optional().nullable(),
});
