import { Checkbox, FormControl, FormControlLabel, FormHelperText, InputLabel, OutlinedInput } from "@mui/material";
import { useController, useFormContext } from "react-hook-form";
import { useFormStyles } from "styles/form";
import { MetricFormValues } from "../MetricForm.types";
import Editor from "react-simple-code-editor";
import { highlight, languages } from "prismjs";
import "prismjs/components/prism-sql";
import "prismjs/themes/prism-tomorrow.css";
import { Controller } from "react-hook-form";

export const MetricFormStepSettings = () => {
  const { register, control } = useFormContext<MetricFormValues>();
  const { classes, cx } = useFormStyles();

  const { field: instanceLevelField } = useController({ name: "IsInstanceLevel", control });

  return (
    <div className={classes.form}>
      <FormControl
        className={cx(classes.formControlInput, classes.widthDefault)}
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
              highlight={(code) => highlight(code, languages.sql, "sql")}
              padding={12}
              id="InitSQL"
              style={{
                fontFamily: "'Fira Code', 'Consolas', monospace",
                fontSize: "0.75rem",
                lineHeight: 1.6,
                minHeight: "120px",
                border: "1px solid rgba(0,0,0,0.23)",
                borderRadius: "4px",
                backgroundColor: "#2d2d2d",
                color: "#ccc",
              }}
            />
          )}
        />
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
