import { FormControl, FormHelperText, InputLabel, OutlinedInput } from "@mui/material";
import cx from "classnames";
import { useFormContext } from "react-hook-form";
import { useFormStyles } from "styles/form";
import { MetricFormValues } from "../MetricForm.types";

export const MetricFormStepGeneral = () => {
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
        className={cx(formClasses.formControlInput, formClasses.widthDefault)}
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
        className={cx(formClasses.formControlInput, formClasses.widthDefault)}
        error={hasError("StorageName")}
        variant="outlined"
      >
        <InputLabel htmlFor="StorageName">Storage name</InputLabel>
        <OutlinedInput
          {...register("StorageName")}
          id="StorageName"
          label="Storage name"
        />
      </FormControl>
      <FormControl
        className={cx(formClasses.formControlInput, formClasses.widthDefault)}
        error={hasError("NodeStatus")}
        variant="outlined"
      >
        <InputLabel htmlFor="NodeStatus">Node status</InputLabel>
        <OutlinedInput
          {...register("NodeStatus")}
          id="NodeStatus"
          label="Node status"
        />
      </FormControl>
      <FormControl
        className={cx(formClasses.formControlInput, formClasses.widthFull)}
        error={hasError("Description")}
        variant="outlined"
      >
        <InputLabel htmlFor="Description">Description</InputLabel>
        <OutlinedInput
          {...register("Description", { required: "Description is required" })}
          id="Description"
          label="Description"
          multiline
          maxRows={2}
        />
      </FormControl>
    </div>
  );
};
