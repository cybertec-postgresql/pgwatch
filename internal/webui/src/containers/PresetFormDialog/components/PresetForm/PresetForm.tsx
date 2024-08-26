import { useState } from "react";
import { useFormStyles } from "styles/form";
import { PresetFormSteps } from "./PresetForm.consts";
import { PresetFormStep } from "./PresetForm.types";
import { PresetFormStepGeneral } from "./components/PresetFormStepGeneral";
import { PresetFormStepMetrics } from "./components/PresetFormStepMetrics";
import { StepButtons } from "./components/StepButtons/StepButtons";

export const PresetForm = () => {
  const [currentStep, setCurrentStep] = useState<PresetFormStep>(PresetFormSteps.General);
  const { classes } = useFormStyles();

  return (
    <div className={classes.formContent}>
      <StepButtons currentStep={currentStep} setCurrentStep={setCurrentStep} />
      {currentStep === PresetFormSteps.General && (
        <PresetFormStepGeneral />
      )}
      {currentStep === PresetFormSteps.Metrics && (
        <PresetFormStepMetrics />
      )}
    </div>
  );
};
