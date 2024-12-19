import { SourceFormSteps } from "../../SourceForm.consts";

export const formErrors = {
  [SourceFormSteps.General]: ["Name", "Group", "ConnStr", "Kind"],
  [SourceFormSteps.Metrics]: ["Metrics", "MetricsStandby", "PresetMetrics"],
  [SourceFormSteps.Tags]: ["CustomTags"],
};
