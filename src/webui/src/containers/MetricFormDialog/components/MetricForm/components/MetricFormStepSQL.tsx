import { Box, FormControl, FormHelperText, InputLabel, OutlinedInput } from "@mui/material";
import { useFormContext } from "react-hook-form";
import { 
  form, 
  formControlInput, 
  widthFull 
 } from "styles/form";
import { MetricFormValues } from "../MetricForm.types";

export const MetricFormStepSQL = () => {
  const { register, formState: { errors } } = useFormContext<MetricFormValues>();

  const hasError = (field: keyof MetricFormValues) => !!errors[field];

  const getError = (field: keyof MetricFormValues) => {
    const error = errors[field];
    if (error) {
      return error.message;
    }
    return undefined;
  };

  return (
    <Box sx={form}>
      <FormControl
        sx={{...formControlInput, ...widthFull}}
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
    </Box>
  );
};
