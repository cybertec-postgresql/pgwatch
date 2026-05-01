import { Checkbox, FormControl, FormControlLabel, FormHelperText, InputLabel, OutlinedInput, useTheme } from "@mui/material";
import { Controller, useController, useFormContext } from "react-hook-form";
import { useFormStyles } from "styles/form";
import { MetricFormValues } from "../MetricForm.types";
import Editor from "react-simple-code-editor";
import { highlight, languages } from "prismjs";


export const MetricFormStepSettings = () => {
  const { register, control, formState: { errors }, } = useFormContext<MetricFormValues>();
  const { classes, cx } = useFormStyles();
  const theme = useTheme();
    const hasError = !!errors.SQLs;
  const errorMessage = errors.SQLs?.message;
  const { field: instanceLevelField } = useController({ name: "IsInstanceLevel", control });

  return (
    <div className={classes.form}>
      <FormControl
        className={cx(classes.formControlInput, classes.widthDefault)}
        variant="outlined"
        error={hasError}
        aria-describedby="InitSQL-error"
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
        className={cx(classes.formControlInput, classes.widthFull)}
         variant="outlined"
      >
         <InputLabel
          shrink
          htmlFor="InitSQL"
          sx={{ backgroundColor: "white", px: 0.5 }}
        >
          Init SQL
        </InputLabel>
         <Controller
          name="InitSQL"
          control={control}
          render={({ field }) => (
              <Editor
                value={field.value ?? ""}
                onValueChange={field.onChange}
                onBlur={field.onBlur} 
                highlight={(code) => highlight(code, languages.sql, "sql")}
                padding={12}
                id="InitSQL"
                style={{
                  fontFamily: "'Fira Code', 'Consolas', monospace",
                  fontSize: "0.75rem",
                  lineHeight: 1.6,
                  minHeight: "120px",
                  borderRadius: "4px",
                  backgroundColor: theme.palette.background.paper,
                  color: theme.palette.text.primary,
                  border: hasError
                  ? `1px solid ${theme.palette.error.main}`
                  : `1px solid ${theme.palette.divider}`,
                }}
              />
            )}
        />
        <FormHelperText id="InitSQL-error">{errorMessage}</FormHelperText>
      </FormControl>
      <FormControlLabel
        className={classes.formControlCheckbox}
        label="Is instance level"
        labelPlacement="start"
        control={
          <Checkbox
            {...instanceLevelField}
            size="medium"
            checked={instanceLevelField.value}
          />
        }
      />
    </div>
  );
};
