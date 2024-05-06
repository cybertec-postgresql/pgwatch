import * as Yup from "yup";

export enum PresetFormSteps {
  General = "General",
  Metrics = "Metrics",
};

const metricValidationSchema = Yup.object({
  Name: Yup.string().required("Choose metric or delete it"),
  Interval: Yup.number().typeError("Please provide a number")
    .required("Interval is required")
    .min(10, "Min. interval is 10")
    .max(604800, "Max. interval is 604800"),
});

export const presetFormValuesValidationSchema = Yup.object({
  Name: Yup.string().required("Name is required"),
  Description: Yup.string().optional(),
  Metrics: Yup.array().of(metricValidationSchema).test((arr, context) => {
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
  }).required("Metrics are required"),
});
