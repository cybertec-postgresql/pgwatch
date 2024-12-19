import { PresetFormSteps } from "../../PresetForm.consts";

export const formErrors = {
  [PresetFormSteps.General]: ["Name"],
  [PresetFormSteps.Metrics]: ["Metrics", "Name", "Interval"],
};
