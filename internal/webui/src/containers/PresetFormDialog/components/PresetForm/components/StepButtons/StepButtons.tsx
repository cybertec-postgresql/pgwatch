import { ToggleButton, ToggleButtonGroup } from "@mui/material";
import { useFormContext } from "react-hook-form";
import { PresetFormSteps } from "../../PresetForm.consts";
import { PresetFormStep, PresetFormValues } from "../../PresetForm.types";
import { formErrors } from "./StepButtons.consts";

type Props = {
  currentStep: PresetFormStep;
  setCurrentStep: React.Dispatch<React.SetStateAction<PresetFormStep>>;
};

export const StepButtons = (props: Props) => {
  const { currentStep, setCurrentStep } = props;

  const { formState: { errors } } = useFormContext<PresetFormValues>();

  const handleStepChange = (_e: any, value?: PresetFormStep) => {
    value && setCurrentStep(value);
  };

  const isStepError = (step: PresetFormStep) => {
    const fields = formErrors[step];
    return Object.keys(errors).some(error => fields.includes(error));
  };

  const buttons = Object.entries(PresetFormSteps).map(([value, label]) => (
    <ToggleButton
      fullWidth
      key={value}
      value={value}
      {...(isStepError(value as PresetFormStep) && {
        color: "error",
        selected: true,
      })}
    >
      {label}
    </ToggleButton>
  ));

  return(
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
