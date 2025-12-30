import { useEffect, useMemo } from "react";
import DeleteIcon from "@mui/icons-material/Delete";
import { Button, Checkbox, FormControl, FormControlLabel, FormHelperText, IconButton, InputLabel, OutlinedInput } from "@mui/material";
import { Autocomplete } from "components/Autocomplete/Autocomplete";
import { Controller, useController, useFieldArray, useFormContext } from "react-hook-form";
import { useFormStyles } from "styles/form";
import { useMetrics } from "queries/Metric";
import { usePresets } from "queries/Preset";
import { SourceFormValues } from "../SourceForm.types";

export const SourceFormStepMetrics = () => {
  const { control, register, watch, formState: { errors }, clearErrors } = useFormContext<SourceFormValues>();
  const metricsFields = useFieldArray({ control, name: "Metrics" });
  const metricsStandbyFields = useFieldArray({ control, name: "MetricsStandby" });

  const { classes, cx } = useFormStyles();

  const { field: onlyIfMasterField } = useController({ control, name: "OnlyIfMaster" });

  const presetMetrics = watch("PresetMetrics");
  const presetMetricsStandby = watch("PresetMetricsStandby");

  useEffect(() => {
    if (presetMetrics) {
      metricsFields.remove();
    }
  }, [presetMetrics, metricsFields]);

  useEffect(() => {
    if (presetMetricsStandby) {
      metricsStandbyFields.remove();
    }
  }, [presetMetricsStandby, metricsStandbyFields]);

  const presets = usePresets();
  const metrics = useMetrics();

  const presetsOptions = useMemo(
    () => presets.data ? Object.keys(presets.data).sort((a, b) => a.localeCompare(b)).map((key) => ({ label: key })) : [],
    [presets.data],
  );

  const metricsOptions = useMemo(
    () => metrics.data ? Object.keys(metrics.data).sort((a, b) => a.localeCompare(b)).map((key) => ({ label: key })) : [],
    [metrics.data],
  );

  const hasError = (field: keyof SourceFormValues) => !!errors[field];

  const getError = (field: keyof SourceFormValues) => {
    const error = errors[field];
    return error && error.message;
  };

  const getMetricsError = (field: "Name" | "Value", index: number, isStandby = false) => {
    const metricsErrors = isStandby ? errors.MetricsStandby : errors.Metrics;
    return metricsErrors && (field === "Name" ? metricsErrors[index]?.Name?.message : metricsErrors[index]?.Value?.message);
  };

  const handleMetricsAppend = () => {
    metricsFields.append({ Name: "", Value: 10 });
    clearErrors("PresetMetrics");
  };

  return (
    <>
      <div className={classes.form}>
        {/* Add typography h4 "Metrics" */}
        <FormControl
          className={cx(classes.formControlInput, classes.widthDefault)}
          error={hasError("PresetMetrics")}
          variant="outlined"
        >
          <Controller
            control={control}
            name="PresetMetrics"
            render={({ field }) => (
              <Autocomplete
                {...field}
                id="PresetMetrics"
                label="Metrics preset"
                options={presetsOptions}
                loading={presets.isLoading}
                error={hasError("PresetMetrics")}
              />
            )}
          />
          <FormHelperText>{getError("PresetMetrics")}</FormHelperText>
        </FormControl>
        {metricsFields.fields.map(({ id }, index) => (
          <div className={classes.row} key={id}>
            <FormControl
              className={cx(classes.formControlInput, classes.widthDefault)}
              error={!!getMetricsError("Name", index)}
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
                    options={metricsOptions}
                    loading={metrics.isLoading}
                    error={!!getMetricsError("Name", index)}
                  />
                )}
              />
              <FormHelperText>{getMetricsError("Name", index)}</FormHelperText>
            </FormControl>
            <FormControl
              className={cx(classes.formControlInput, classes.widthDefault)}
              error={!!getMetricsError("Value", index)}
              variant="outlined"
            >
              <InputLabel htmlFor={`Metrics.${index}.Value`}>Value</InputLabel>
              <OutlinedInput
                {...register(`Metrics.${index}.Value`)}
                id={`Metrics.${index}.Value`}
                label="Value"
                type="number"
                endAdornment={
                  <IconButton
                    key={`Metrics.${index}.Delete`}
                    title="Delete metric"
                    onClick={() => metricsFields.remove(index)}
                  >
                    <DeleteIcon />
                  </IconButton>
                }
              />
              <FormHelperText>{getMetricsError("Value", index)}</FormHelperText>
            </FormControl>
          </div>
        ))}
        <div className={cx(classes.row, classes.addButton)}>
          <Button
            variant="contained"
            onClick={handleMetricsAppend}
            disabled={!!presetMetrics}
          >
            Add metric
          </Button>
        </div>
      </div>
      <div className={classes.form}>
        {/* Add typography h4 "Metrics standby" */}
        <FormControl
          className={cx(classes.formControlInput, classes.widthDefault)}
          variant="outlined"
        >
          <Controller
            control={control}
            name="PresetMetricsStandby"
            render={({ field }) => (
              <Autocomplete
                {...field}
                id="PresetMetricsStandby"
                label="Metrics standby preset"
                options={presetsOptions}
                loading={presets.isLoading}
              />
            )}
          />
        </FormControl>
        {metricsStandbyFields.fields.map(({ id }, index) => (
          <div className={classes.row} key={id}>
            <FormControl
              className={cx(classes.formControlInput, classes.widthDefault)}
              error={!!getMetricsError("Name", index, true)}
              variant="outlined"
            >
              <Controller
                name={`MetricsStandby.${index}.Name`}
                control={control}
                render={({ field }) => (
                  <Autocomplete
                    {...field}
                    id={`MetricsStandby.${index}.Name`}
                    label="Name"
                    options={metricsOptions}
                    loading={metrics.isLoading}
                    error={!!getMetricsError("Name", index, true)}
                  />
                )}
              />
              <FormHelperText>{getMetricsError("Name", index, true)}</FormHelperText>
            </FormControl>
            <FormControl
              className={cx(classes.formControlInput, classes.widthDefault)}
              error={!!getMetricsError("Value", index, true)}
              variant="outlined"
            >
              <InputLabel htmlFor={`MetricsStandby.${index}.Value`}>Value</InputLabel>
              <OutlinedInput
                {...register(`MetricsStandby.${index}.Value`)}
                id={`MetricsStandby.${index}.Value`}
                label="Value"
                type="number"
                endAdornment={
                  <IconButton
                    key={`MetricsStandby.${index}.Delete`}
                    title="Delete metric"
                    onClick={() => metricsStandbyFields.remove(index)}
                  >
                    <DeleteIcon />
                  </IconButton>
                }
              />
              <FormHelperText>{getMetricsError("Value", index, true)}</FormHelperText>
            </FormControl>
          </div>
        ))}
        <div className={cx(classes.row, classes.addButton)}>
          <Button
            variant="contained"
            onClick={() => metricsStandbyFields.append({ Name: "", Value: 10 })}
            disabled={!!presetMetricsStandby}
          >
            Add metric
          </Button>
        </div>
        <FormControlLabel
          className={classes.formControlCheckbox}
          label="Primary mode only"
          labelPlacement="start"
          control={
            <Checkbox
              {...onlyIfMasterField}
              size="medium"
              checked={onlyIfMasterField.value} 
            />
          }
        />
      </div>
    </>
  );
};
