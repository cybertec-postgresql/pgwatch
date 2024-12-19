import { ToggleButton, ToggleButtonGroup } from "@mui/material";
import { useFormContext } from "react-hook-form";
import { SourceFormSteps } from "../../SourceForm.consts";
import { SourceFormStep, SourceFormValues } from "../../SourceForm.types";
import { formErrors } from "./StepButtons.consts";

type Props = {
  currentStep: SourceFormStep;
  setCurrentStep: React.Dispatch<React.SetStateAction<SourceFormStep>>;
};

export const StepButtons = (props: Props) => {
  const { currentStep, setCurrentStep } = props;

  const { formState: { errors } } = useFormContext<SourceFormValues>();

  const handleStepChange = (_e: any, value?: SourceFormStep) => {
    value && setCurrentStep(value);
  };

  const isStepError = (step: SourceFormStep) => {
    const fields = formErrors[step];
    return Object.keys(errors).some(error => fields.includes(error));
  };

  const buttons = Object.entries(SourceFormSteps).map(([value, label]) => (
    <ToggleButton
      fullWidth
      key={value}
      value={value}
      {...(isStepError(value as SourceFormStep) && {
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
