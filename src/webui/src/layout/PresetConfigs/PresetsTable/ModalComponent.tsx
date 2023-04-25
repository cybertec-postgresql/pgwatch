import { useState } from "react";

import CloseIcon from "@mui/icons-material/Close";
import DoneIcon from "@mui/icons-material/Done";
import { AlertColor, Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, Stack, TextField, ToggleButton, ToggleButtonGroup } from "@mui/material";

import { Controller, FormProvider, SubmitHandler, useForm, useFormContext } from "react-hook-form";

import { useAddPreset } from "queries/Preset";
import { CreatePresetConfigForm, CreatePresetConfigRequestForm, Preset } from "queries/types/PresetTypes";

import { AddMetric } from "./AddMetric";


type Props = {
  open: boolean;
  handleClose: () => void;
  recordData: Preset | undefined;
  handleAlertOpen: (text: string, type: AlertColor) => void;
};

export const ModalComponent = ({ open, handleClose, recordData, handleAlertOpen }: Props) => {
  const methods = useForm<CreatePresetConfigForm>({
    defaultValues: {
      pc_config: [{ metric: "", update_interval: 10 }]
    }
  });
  const { handleSubmit, reset } = methods;
  const addPreset = useAddPreset(handleAlertOpen, handleClose, reset);

  const requestedForm = (result: CreatePresetConfigForm): CreatePresetConfigRequestForm => {
    const config: Record<string, number> = {};
    result.pc_config.map(({ metric, update_interval }) => config[metric] = update_interval);
    return { ...result, pc_config: config };
  };

  const onSubmit: SubmitHandler<CreatePresetConfigForm> = (result) => {
    addPreset.mutate(requestedForm(result));
  };

  return (
    <Dialog
      onClose={handleClose}
      open={open}
      fullWidth
      maxWidth="md"
    >
      <DialogTitle>{recordData ? "Edit preset config" : "Add new preset config"}</DialogTitle>
      <FormProvider {...methods}>
        <form onSubmit={handleSubmit(onSubmit)}>
          <DialogContent>
            <ModalContent />
          </DialogContent>
          <DialogActions>
            <Button fullWidth onClick={handleClose} size="medium" variant="outlined" startIcon={<CloseIcon />}>Cancel</Button>
            <Button type="submit" fullWidth size="medium" variant="contained" startIcon={<DoneIcon />}>{recordData ? "Submit changes" : "Create preset"}</Button>
          </DialogActions>
        </form>
      </FormProvider>
    </Dialog>
  );
};

enum Steps {
  main = "Main",
  metrics = "Metrics"
};

type StepType = keyof typeof Steps;
const defaultStep = Object.keys(Steps)[0] as StepType;

const formErrors = {
  main: ["pc_name", "pc_description"],
  metrics: ["pc_config"]
};

const getStepError = (step: StepType, errors: string[]): boolean => {
  const fields: string[] = formErrors[step];
  return errors.some(error => fields.includes(error));
};

const ModalContent = () => {
  const { control, formState: { errors } } = useFormContext<CreatePresetConfigForm>();
  const [activeStep, setActiveStep] = useState<StepType>(defaultStep);

  const handleValidate = (val: string | number) => !!val.toString().trim();

  const stepContent = {
    main: (
      <Stack spacing={2}>
        <Controller
          name="pc_name"
          control={control}
          rules={{
            required: {
              value: true,
              message: "Name is required"
            },
            validate: handleValidate
          }}
          defaultValue=""
          render={({ field, fieldState: { error } }) => (
            <TextField
              {...field}
              error={!!error}
              helperText={error?.message}
              type="text"
              label="Name"
              fullWidth
            />
          )}
        />
        <Controller
          name="pc_description"
          control={control}
          rules={{
            required: {
              value: true,
              message: "Description is required"
            },
            validate: handleValidate
          }}
          defaultValue=""
          render={({ field, fieldState: { error } }) => (
            <TextField
              {...field}
              error={!!error}
              helperText={error?.message}
              type="text"
              label="Description"
              multiline
              minRows={2}
              maxRows={4}
              fullWidth
            />
          )}
        />
      </Stack>
    ),
    metrics: (
      <AddMetric control={control} handleValidate={handleValidate} />
    )
  };

  const handleChange = (_ev: any, value?: StepType) => {
    if (value) {
      setActiveStep(value);
    }
  };

  const buttons = Object.entries(Steps).map(([val, label]) => (
    <ToggleButton
      fullWidth
      key={val}
      value={val}
      {...(getStepError(val as StepType, Object.keys(errors ?? {})) && {
        color: "error",
        selected: true
      })}
    >
      {label}
    </ToggleButton>
  ));

  const content = Object.keys(Steps).map((key) => (
    <Box
      key={`Content-${key}`}
      {...(key !== activeStep && {
        height: 0,
        overflow: "hidden"
      })}
    >
      <>{stepContent[key as StepType]}</>
    </Box>
  ));

  return (
    <>
      <ToggleButtonGroup
        color="primary"
        value={activeStep}
        exclusive
        onChange={handleChange}
        aria-label="Platform"
        fullWidth
      >
        {buttons}
      </ToggleButtonGroup>
      <Box
        pt={2}
      >
        <>{content}</>
      </Box>
    </>
  );
};
