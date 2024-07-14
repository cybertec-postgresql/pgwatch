import { useState } from "react";
import { useFormStyles } from "styles/form";
import { SourceFormSteps } from "./SourceForm.consts";
import { SourceFormStep } from "./SourceForm.types";
import { SourceFormStepGeneral } from "./components/SourceFormStepGeneral";
import { SourceFormStepMetrics } from "./components/SourceFormStepMetrics";
import { SourceFormStepTags } from "./components/SourceFormStepTags";
import { StepButtons } from "./components/StepButtons/StepButtons";

export const SourceForm = () => {
  const [currentStep, setCurrentStep] = useState<SourceFormStep>(SourceFormSteps.General);
  const { classes } = useFormStyles();

  return (
    <div className={classes.formContent}>
      <StepButtons currentStep={currentStep} setCurrentStep={setCurrentStep} />
      {currentStep === SourceFormSteps.General && <SourceFormStepGeneral />}
      {currentStep === SourceFormSteps.Metrics && <SourceFormStepMetrics />}
      {currentStep === SourceFormSteps.Tags && <SourceFormStepTags />}
    </div>
  );
};
