import { MetricFormSteps } from "../../MetricForm.consts";

export const formErrors = {
  [MetricFormSteps.General]: ["Name"],
  [MetricFormSteps.Settings]: [""],
  [MetricFormSteps.SQLs]: ["SQLs"],
};
