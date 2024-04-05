import { FormControl, FormHelperText, InputLabel, OutlinedInput } from "@mui/material";
import cx from "classnames";
import { useFormContext } from "react-hook-form";
import { useFormStyles } from "styles/form";
import { MetricFormValues } from "../MetricForm.types";

export const MetricFormStepSQL = () => {
  const { register, formState: { errors } } = useFormContext<MetricFormValues>();

  const formClasses = useFormStyles();

  const hasError = (field: keyof MetricFormValues) => !!errors[field];

  const getError = (field: keyof MetricFormValues) => {
    const error = errors[field];
    if (error) {
      return error.message;
    }
    return undefined;
  };

  return (
    <div className={formClasses.form}>
      <FormControl
        className={cx(formClasses.formControlInput, formClasses.widthFull)}
        error={hasError("SQLs")}
        variant="outlined"
      >
        <InputLabel htmlFor="SQLs">SQLs</InputLabel>
        <OutlinedInput
          {...register("SQLs")}
          id="SQLs"
          label="SQLs"
          aria-describedby="SQLs-error"
          multiline
          rows={15}
          inputProps={{
            style: {
              font: "revert",
              fontSize: "0.7rem",
            }
          }}
        />
        <FormHelperText id="SQLs-error">{getError("SQLs")}</FormHelperText>
      </FormControl>
    </div>
  );
};
