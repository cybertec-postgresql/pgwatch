import { PresetGridRow } from "pages/PresetsPage/components/PresetsGrid/PresetsGrid.types";
import { PresetRequestBody } from "types/Preset/PresetRequestBody";
import { PresetFormValues } from "./components/PresetForm/PresetForm.types";

export const getPresetInitialValues = (data?: PresetGridRow): PresetFormValues => {
  const metrics = data ?
    Object.keys(data.Preset.Metrics).map((key) => ({
      Name: key,
      Interval: data.Preset.Metrics[key],
    })) : [{ Name: "", Interval: 10 }];

  return {
    Name: data?.Key ?? "",
    Description: data?.Preset.Description ?? "",
    Metrics: metrics,
  };
};

export const createPresetRequest = (values: PresetFormValues): PresetRequestBody => {
  const metrics: Record<string, number> = {};
  values.Metrics.map(({ Name, Interval }) => metrics[Name] = Interval);
  return {
    Name: values.Name,
    Data: {
      Description: values.Description,
      Metrics: metrics,
    },
  };
};
