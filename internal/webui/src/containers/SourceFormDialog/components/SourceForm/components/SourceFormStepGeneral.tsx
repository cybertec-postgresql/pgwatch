import { useMemo } from "react";
import { Checkbox, FormControl, FormControlLabel, FormHelperText, InputLabel, OutlinedInput } from "@mui/material";
import { Autocomplete } from "components/Autocomplete/Autocomplete";
import { Controller, useController, useFormContext } from "react-hook-form";
import { useFormStyles } from "styles/form";
import { KindOptions } from "../SourceForm.consts";
import { SourceFormValues } from "../SourceForm.types";
import { TestConnection } from "./TestConnection/TestConnection";

export const SourceFormStepGeneral = () => {
  const { register, formState: { errors }, control, watch } = useFormContext<SourceFormValues>();
  const { classes, cx } = useFormStyles();

  const kindValue = watch("Kind");
  const connStrValue = watch("ConnStr");

  const isPatternHidden = useMemo(
    () => !kindValue.includes("discovery"),
    [kindValue],
  );

  const hasError = (field: keyof SourceFormValues) => !!errors[field];

  const getError = (field: keyof SourceFormValues) => {
    const error = errors[field];
    return error && error.message;
  };

  const { field: isEnabledField } = useController({ control, name: "IsEnabled" });

  return (
    <div className={classes.form}>
      <FormControl
        className={cx(classes.formControlInput, classes.widthDefault)}
        error={hasError("Name")}
        variant="outlined"
      >
        <InputLabel htmlFor="Name">Unique name</InputLabel>
        <OutlinedInput
          {...register("Name")}
          id="Unique name"
          label="Unique name"
        />
        <FormHelperText>{getError("Name")}</FormHelperText>
      </FormControl>
      <FormControl
        className={cx(classes.formControlInput, classes.widthDefault)}
        error={hasError("Group")}
        variant="outlined"
      >
        <InputLabel htmlFor="Group">Group</InputLabel>
        <OutlinedInput
          {...register("Group")}
          id="Group"
          label="Group"
        />
        <FormHelperText>{getError("Group")}</FormHelperText>
      </FormControl>
      <FormControl
        className={cx(classes.formControlInput, classes.widthFull)}
        error={hasError("ConnStr")}
        variant="outlined"
      >
        <InputLabel htmlFor="ConnStr">Connection string</InputLabel>
        <OutlinedInput
          {...register("ConnStr")}
          id="ConnStr"
          label="Connection string"
          endAdornment={
            <TestConnection ConnStr={connStrValue} />
          }
        />
        <FormHelperText>{getError("ConnStr")}</FormHelperText>
      </FormControl>
      <FormControl
        className={cx(classes.formControlInput, classes.widthDefault)}
        error={hasError("Kind")}
        variant="outlined"
      >
        <Controller
          control={control}
          name="Kind"
          render={({ field }) => (
            <Autocomplete
              {...field}
              id="Kind"
              label="Type"
              options={KindOptions}
              error={hasError("Kind")}
            />
          )}
        />
        <FormHelperText>{getError("Kind")}</FormHelperText>
      </FormControl>
      <FormControl
        className={cx(classes.formControlInput, classes.widthDefault, {
          [classes.hidden]: isPatternHidden,
        })}
        variant="outlined"
      >
        <InputLabel htmlFor="IncludePattern">Include pattern</InputLabel>
        <OutlinedInput
          {...register("IncludePattern")}
          id="IncludePattern"
          label="Include pattern"
        />
      </FormControl>
      <FormControl
        className={cx(classes.formControlInput, classes.widthDefault, {
          [classes.hidden]: isPatternHidden,
        })}
        variant="outlined"
      >
        <InputLabel htmlFor="ExcludePattern">Exclude pattern</InputLabel>
        <OutlinedInput
          {...register("ExcludePattern")}
          id="ExcludePattern"
          label="Exclude pattenr"
        />
      </FormControl>
      <FormControlLabel
        className={classes.formControlCheckbox}
        label="Is enabled"
        labelPlacement="start"
        control={
          <Checkbox
            {...isEnabledField}
            size="medium"
            checked={isEnabledField.value}
          />
        }
      />
    </div>
  );
};
