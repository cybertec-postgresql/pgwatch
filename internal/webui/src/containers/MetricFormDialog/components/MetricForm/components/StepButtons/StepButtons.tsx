import { ToggleButton, ToggleButtonGroup } from "@mui/material";
import { useFormContext } from "react-hook-form";
import { MetricFormSteps } from "../../MetricForm.consts";
import { MetricFormStep, MetricFormValues } from "../../MetricForm.types";
import { formErrors } from "./StepButtons.consts";

type Props = {
  currentStep: MetricFormStep;
  setCurrentStep: React.Dispatch<React.SetStateAction<MetricFormStep>>,
};

export const StepButtons = (props: Props) => {
  const { currentStep, setCurrentStep } = props;

  const { formState: { errors } } = useFormContext<MetricFormValues>();

  const handleStepChange = (_e: any, value?: MetricFormStep) => {
    value && setCurrentStep(value);
  };

  const isStepError = (step: MetricFormStep) => {
    const fields = formErrors[step];
    return Object.keys(errors).some(error => fields.includes(error));
  };

  const buttons = Object.entries(MetricFormSteps).map(([value, label]) => (
    <ToggleButton
      fullWidth
      key={value}
      value={value}
      {...(isStepError(value as MetricFormStep) && {
        color: "error",
        selected: true,
      })}
    >
      {label}
    </ToggleButton>
  ));

  return (
    <ToggleButtonGroup
      color="primary"
      value={currentStep}
      exclusive
      onChange={handleStepChange}
      aria-label="Platform"
      fullWidth
    >
      {buttons}
    </ToggleButtonGroup>
  );
};
