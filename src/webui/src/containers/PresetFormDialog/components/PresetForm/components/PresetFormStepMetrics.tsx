import { useMemo } from "react";
import AddIcon from "@mui/icons-material/Add";
import DeleteIcon from "@mui/icons-material/Delete";
import { FormControl, FormHelperText, IconButton, InputLabel, OutlinedInput, Skeleton } from "@mui/material";
import { Autocomplete } from "components/Autocomplete/Autocomplete";
import { Controller, useFieldArray, useFormContext } from "react-hook-form";
import { useFormStyles } from "styles/form";
import { ErrorComponent } from "layout/common/ErrorComponent";
import { useMetrics } from "queries/Metric";
import { PresetFormValues } from "../PresetForm.types";

export const PresetFormStepMetrics = () => {
  const { control, register, formState: { errors } } = useFormContext<PresetFormValues>();
  const { fields, append, remove } = useFieldArray({
    name: "Metrics",
    control
  });
  const { classes, cx } = useFormStyles();

  const { data, isLoading, isError, error } = useMetrics();

  const options = useMemo(
    () => data ? Object.keys(data).map((name) => ({ label: name })) : [],
    [data],
  );

  const getError = (field: "Name" | "Interval", index: number) => {
    const metricsErrors = errors.Metrics;
    if (!metricsErrors) {
      return undefined;
    }
    return field === "Name" ? metricsErrors[index]?.Name?.message : metricsErrors[index]?.Interval?.message;
  };

  if (isError) {
    const err = error as Error;
    return (
      <ErrorComponent errorMessage={err.message} />
    );
  }

  return (
    <div className={classes.form}>
      {fields.map(({ id }, index) => (
        <div className={classes.row} key={id}>
          <FormControl
            className={cx(classes.formControlInput, classes.widthDefault)}
            error={!!getError("Name", index)}
            variant="outlined"
          >
            <Controller
              name={`Metrics.${index}.Name`}
              control={control}
              render={({ field }) => (
                <Autocomplete
                  {...field}
                  id={`Metrics.${index}.Name`}
                  label="Name"
                  options={options}
                  error={!!getError("Name", index)}
                  loading={isLoading}
                />
              )}
            />
            <FormHelperText>{getError("Name", index)}</FormHelperText>
          </FormControl>
          <FormControl
            className={cx(classes.formControlInput, classes.widthDefault)}
            error={!!getError("Interval", index)}
            variant="outlined"
          >
            <InputLabel htmlFor={`Metrics.${index}.Interval`}>Interval</InputLabel>
            <OutlinedInput
              {...register(`Metrics.${index}.Interval`)}
              id={`Metrics.${index}.Interval`}
              label="Interval"
              type="number"
            />
            <FormHelperText>{getError("Interval", index)}</FormHelperText>
          </FormControl>
          <div className={classes.iconRow}>
            <IconButton
              key={`Metrics.${index}.Delete`}
              title="Delete metric"
              onClick={() => remove(index)}
              {...fields.length === 1 && {
                disabled: true
              }}
            >
              <DeleteIcon />
            </IconButton>
          </div>
        </div>
      ))}
      <div className={classes.row}>
        <Skeleton variant="rounded" width={240} height={56} />
        <Skeleton variant="rounded" width={240} height={56} />
        <div className={classes.iconRow}>
          <IconButton
            title="Add metric"
            onClick={() => append({ Name: "", Interval: 10 })}
          >
            <AddIcon />
          </IconButton>
        </div>
      </div>
    </div>
  );
};
