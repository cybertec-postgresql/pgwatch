import { Box, Checkbox, FormControl, FormControlLabel, FormHelperText, InputLabel, OutlinedInput } from "@mui/material";
import cx from "classnames";
import { useController, useFormContext } from "react-hook-form";
import { 
  formDialog, 
  formContent, 
  form, 
  formControlInput, 
  formControlCheckbox, 
  formButtons, 
  widthDefault, 
  widthFull 
 } from "styles/form";
import { MetricFormValues } from "../MetricForm.types";

export const MetricFormStepSettings = () => {
  const { register, control } = useFormContext<MetricFormValues>();

  const { field } = useController({ name: "IsInstanceLevel", control });

  return (
    <Box sx={form}>
      <FormControl
        sx={{...formControlInput, ...widthDefault}}
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
        sx={{...formControlInput, ...widthFull}}
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
        sx={formControlCheckbox}
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
    </Box>
  );
};
