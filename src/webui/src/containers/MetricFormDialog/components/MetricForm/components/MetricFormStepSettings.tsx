import { Checkbox, FormControl, FormControlLabel, FormHelperText, InputLabel, OutlinedInput } from "@mui/material";
import cx from "classnames";
import { useController, useFormContext } from "react-hook-form";
import { useFormStyles } from "styles/form";
import { MetricFormValues } from "../MetricForm.types";

export const MetricFormStepSettings = () => {
  const { register, control } = useFormContext<MetricFormValues>();

  const { field } = useController({ name: "IsInstanceLevel", control });

  const formClasses = useFormStyles();

  return (
    <div className={formClasses.form}>
      <FormControl
        className={cx(formClasses.formControlInput, formClasses.widthDefault)}
        variant="outlined"
      >
        <InputLabel htmlFor="Gauges">Gauges</InputLabel>
        <OutlinedInput
          {...register("Gauges")}
          id="Gauges"
          label="Gauges"
          aria-describedby="Gauges-helper"
          multiline
          maxRows={3}
        />
        <FormHelperText id="Gauges-helper">Write every gauge with a new line</FormHelperText>
      </FormControl>
      <FormControl
        className={cx(formClasses.formControlInput, formClasses.widthFull)}
        variant="outlined"
      >
        <InputLabel htmlFor="InitSQL">Init SQL</InputLabel>
        <OutlinedInput
          {...register("InitSQL")}
          id="InitSQL"
          label="Init SQL"
          multiline
          maxRows={5}
        />
      </FormControl>
      <FormControlLabel
        className={formClasses.formControlCheckbox}
        label="Is instance level"
        labelPlacement="start"
        control={
          <Checkbox
            {...field}
            size="medium"
            checked={field.value}
          />
        }
      />
    </div>
  );
};
