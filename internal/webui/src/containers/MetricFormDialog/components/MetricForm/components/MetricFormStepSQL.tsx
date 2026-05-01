import { FormControl, FormHelperText, InputLabel, OutlinedInput, useTheme } from "@mui/material";
import { Controller, useFormContext } from "react-hook-form";
import { useFormStyles } from "styles/form";
import { MetricFormValues } from "../MetricForm.types";
import Editor from "react-simple-code-editor";
import { highlight, languages } from "prismjs";



export const MetricFormStepSQL = () => {
  const { control, formState: { errors } } = useFormContext<MetricFormValues>();
  const { classes, cx } = useFormStyles();
  const theme = useTheme();

  const hasError = !!errors.SQLs;
  const errorMessage = errors.SQLs?.message;

  return (
    <div className={classes.form}>
      <FormControl
        className={cx(classes.formControlInput, classes.widthFull)}
        error={hasError}
        variant="outlined"
        aria-describedby="SQLs-error"
      >
        <InputLabel
          shrink
          htmlFor="SQLs"
          sx={{ backgroundColor: "white", px: 0.5 }}
        >
          SQLs
        </InputLabel>
         <Controller
          name="SQLs"
          control={control}
          render={({ field }) => (
            <Editor
              value={field.value ?? ""}
              onValueChange={field.onChange}
              onBlur={field.onBlur} 
              highlight={(code) => highlight(code, languages.sql, "sql")}
              padding={12}
              id="InitSQL"
              aria-describedby="InitSQL-error"
              style={{
                fontFamily: "'Fira Code', 'Consolas', monospace",
                fontSize: "0.75rem",
                lineHeight: 1.6,
                minHeight: "240px",
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
        <FormHelperText id="SQLs-error">{errorMessage}</FormHelperText>
      </FormControl>
    </div>
  );
};
