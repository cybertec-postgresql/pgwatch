import { useEffect, useState } from "react";

import CloseIcon from "@mui/icons-material/Close";
import DoneIcon from "@mui/icons-material/Done";
import { ToggleButton } from "@mui/lab";
import { Box, Button, Checkbox, Dialog, DialogActions, DialogContent, DialogTitle, FormControlLabel, Stack, TextField, ToggleButtonGroup } from "@mui/material";

import { Controller, FieldPath, FormProvider, SubmitHandler, useForm, useFormContext } from "react-hook-form";

import { useNavigate } from "react-router-dom";

import { useAddMetric, useEditMetric } from "queries/Metric";
import { Metric, createMetricForm } from "queries/types/MetricTypes";

type Params = {
  recordData: Metric | undefined,
  open: boolean,
  handleClose: () => void
}

export const ModalComponent = ({ recordData, open, handleClose }: Params) => {
  const navigate = useNavigate();
  const methods = useForm<createMetricForm>();
  const { handleSubmit, reset, setValue, clearErrors } = methods;

  useEffect(() => {
    if (recordData) {
      clearErrors();
      Object.entries(recordData).map(([key, value]) => setValue(key as FieldPath<createMetricForm>, convertValue(value)));
    } else {
      reset();
    }
  }, [recordData, setValue, reset, clearErrors]);

  const convertValue = (value: any): any => {
    if (typeof value === "object" && value !== null) {
      return JSON.stringify(value);
    } else {
      return value;
    }
  };

  const addMetric = useAddMetric(handleClose, reset);

  const editMetric = useEditMetric(handleClose, reset);

  const onSubmit: SubmitHandler<createMetricForm> = (result) => {
    if (recordData) {
      editMetric.mutate({
        m_id: recordData.m_id,
        data: result
      });
    } else {
      addMetric.mutate(result);
    }
  };

  return (
    <Dialog
      open={open}
      onClose={handleClose}
      fullWidth
      maxWidth="md"
    >
      <DialogTitle>{recordData ? "Edit metric" : "Add new metric"}</DialogTitle>
      <FormProvider {...methods}>
        <form onSubmit={handleSubmit(onSubmit)}>
          <DialogContent>
            <ModalContent />
          </DialogContent>
          <DialogActions>
            <Button fullWidth onClick={handleClose} size="medium" variant="outlined" startIcon={<CloseIcon />}>Cancel</Button>
            <Button fullWidth type="submit" size="medium" variant="contained" startIcon={<DoneIcon />}>Submit</Button>
          </DialogActions>
        </form>
      </FormProvider>
    </Dialog>
  );
};

enum Steps {
  main = "Main",
  sql = "SQL"
};

type StepType = keyof typeof Steps;
const defaultStep = Object.keys(Steps)[0] as StepType;

const formErrors = {
  main: ["m_name", "m_pg_version_from"],
  sql: ["m_sql"]
};

const getStepError = (step: StepType, errors: string[]): boolean => {
  const fields: string[] = formErrors[step];
  return errors.some(error => fields.includes(error));
};

const ModalContent = () => {
  const { control, formState: { errors } } = useFormContext();
  const [activeStep, setActiveStep] = useState<StepType>(defaultStep);

  const handleValidate = (val: string) => !!val.toString().trim();

  const stepContent = {
    main: (
      <Stack spacing={2}>
        <Stack direction="row" spacing={1}>
          <Controller
            name="m_name"
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
                title="Metric name. Lowercase alphanumerics and underscores allowed."
                fullWidth
              />
            )}
          />
          <Controller
            name="m_pg_version_from"
            control={control}
            rules={{
              required: {
                value: true,
                message: "PG version from is required"
              },
              validate: handleValidate
            }}
            defaultValue=""
            render={({ field, fieldState: { error } }) => (
              <TextField
                {...field}
                error={!!error}
                helperText={error?.message}
                type="number"
                label="PG version from"
                InputProps={{
                  inputProps: {
                    min: 1,
                    step: "0.1"
                  }
                }}
                title="Version from"
                fullWidth
              />
            )}
          />
        </Stack>
        <Stack direction="row" spacing={1}>
          <Controller
            name="m_comment"
            control={control}
            defaultValue=""
            render={({ field }) => (
              <TextField
                {...field}
                type="text"
                label="Comment"
                title="Comment"
                fullWidth
              />
            )}
          />
          <Controller
            name="m_column_attrs"
            control={control}
            defaultValue={null}
            render={({ field }) => (
              <TextField
                {...field}
                type="text"
                label="Column attributes"
                title="Column attributes. Use to specify Prometheus Gauge type columns. 'Gauge' means non-cumulative columns."
                fullWidth
              />
            )}
          />
        </Stack>
        <Stack direction="row" spacing={1} sx={{ justifyContent: "center" }}>
          <Controller
            name="m_is_active"
            control={control}
            defaultValue={true}
            render={({ field }) => (
              <FormControlLabel
                label="Is active?"
                labelPlacement="end"
                control={
                  <Checkbox
                    {...field}
                    size="medium"
                    checked={field.value}
                  />
                }
              />
            )}
          />
          <Controller
            name="m_is_helper"
            control={control}
            defaultValue={false}
            render={({ field }) => (
              <FormControlLabel
                label="Is helper?"
                labelPlacement="end"
                control={
                  <Checkbox
                    {...field}
                    size="medium"
                    checked={field.value}
                  />
                }
              />
            )}
          />
          <Controller
            name="m_master_only"
            control={control}
            defaultValue={false}
            render={({ field }) => (
              <FormControlLabel
                label="Master only?"
                labelPlacement="end"
                control={
                  <Checkbox
                    {...field}
                    size="medium"
                    checked={field.value}
                  />
                }
              />
            )}
          />
          <Controller
            name="m_standby_only"
            control={control}
            defaultValue={false}
            render={({ field }) => (
              <FormControlLabel
                label="Standby only?"
                labelPlacement="end"
                control={
                  <Checkbox
                    {...field}
                    size="medium"
                    checked={field.value}
                  />
                }
              />
            )}
          />
        </Stack>
      </Stack>
    ),
    sql: (
      <Stack spacing={2}>
        <Controller
          name="m_sql"
          control={control}
          rules={{
            required: {
              value: true,
              message: "Sql is required"
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
              label="SQL"
              multiline
              minRows={5}
              maxRows={5}
              title="SQL for metric"
              fullWidth
            />
          )}
        />
        <Controller
          name="m_sql_su"
          control={control}
          defaultValue=""
          render={({ field }) => (
            <TextField
              {...field}
              type="text"
              label="SQL superuser"
              multiline
              minRows={5}
              maxRows={5}
              title="Privileged (superuser or pg_monitor grant) SQL for metric"
              fullWidth
            />
          )}
        />
      </Stack>
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
        minHeight="26vh"
      >
        <>{content}</>
      </Box>
    </>
  );
};
