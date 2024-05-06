import { FormControl, FormHelperText, InputLabel, OutlinedInput } from "@mui/material";
import { useFormContext } from "react-hook-form";
import { useFormStyles } from "styles/form";
import { PresetFormValues } from "../PresetForm.types";

export const PresetFormStepGeneral = () => {
  const { register, formState: { errors } } = useFormContext<PresetFormValues>();
  const { classes, cx } = useFormStyles();

  const hasError = (field: keyof PresetFormValues) => !!errors[field];

  const getError = (field: keyof PresetFormValues) => {
    const error = errors[field];
    return error && error.message;
  };

  return (
    <div className={classes.form}>
      <FormControl
        className={cx(classes.formControlInput, classes.widthDefault)}
        error={hasError("Name")}
        variant="outlined"
      >
        <InputLabel htmlFor="Name">Name</InputLabel>
        <OutlinedInput
          {...register("Name")}
          id="Name"
          label="Name"
          aria-describedby="Name-error"
        />
        <FormHelperText id="Name-error">{getError("Name")}</FormHelperText>
      </FormControl>
      <FormControl
        className={cx(classes.formControlInput, classes.widthFull)}
        variant="outlined"
      >
        <InputLabel htmlFor="Description">Description</InputLabel>
        <OutlinedInput
          {...register("Description")}
          id="Description"
          label="Description"
          multiline
          maxRows={2}
        />
      </FormControl>
    </div>
  );
};
