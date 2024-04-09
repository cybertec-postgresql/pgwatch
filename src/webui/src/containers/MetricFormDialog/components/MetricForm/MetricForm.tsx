import { useState } from "react";
import { Box } from "@mui/material";
import { formContent } from "styles/form";
import { MetricFormSteps } from "./MetricForm.consts";
import { MetricFormStep } from "./MetricForm.types";
import { MetricFormStepGeneral } from "./components/MetricFormStepGeneral";
import { MetricFormStepSQL } from "./components/MetricFormStepSQL";
import { MetricFormStepSettings } from "./components/MetricFormStepSettings";
import { StepButtons } from "./components/StepButtons/StepButtons";

export const MetricForm = () => {
  const [currentStep, setCurrentStep] = useState<MetricFormStep>(MetricFormSteps.General);

  return (
    <Box sx={formContent}>
      <StepButtons currentStep={currentStep} setCurrentStep={setCurrentStep} />
      {currentStep === MetricFormSteps.General && (
        <MetricFormStepGeneral />
      )}
      {currentStep === MetricFormSteps.Settings && (
        <MetricFormStepSettings />
      )}
      {currentStep === MetricFormSteps.SQLs && (
        <MetricFormStepSQL />
      )}
    </Box>
  );
};
