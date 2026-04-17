import { FormControl, FormHelperText, InputLabel, OutlinedInput } from "@mui/material";
import { useFormContext } from "react-hook-form";
import { useFormStyles } from "styles/form";
import { MetricFormValues } from "../MetricForm.types";
import Editor from "react-simple-code-editor";
import { highlight, languages } from "prismjs";
import "prismjs/components/prism-sql";
import "prismjs/themes/prism.css";
import { Controller } from "react-hook-form";


export const MetricFormStepSQL = () => {
  const { control, formState: { errors } } = useFormContext<MetricFormValues>();
  const { classes, cx } = useFormStyles();

  const hasError = !!errors.SQLs;
  const errorMessage = errors.SQLs?.message;

  return (
    <div className={classes.form}>
      <FormControl
        className={cx(classes.formControlInput, classes.widthFull)}
        error={hasError}
        variant="outlined"
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
              highlight={(code) => highlight(code, languages.sql, "sql")}
              padding={12}
              id="SQLs"
              style={{
                fontFamily: "'Fira Code', 'Consolas', monospace",
                fontSize: "0.75rem",
                lineHeight: 1.6,
                minHeight: "240px",
                border: hasError ? "1px solid #d32f2f" : "1px solid rgba(0,0,0,0.23)",
                borderRadius: "4px",
                backgroundColor: "#fafafa",
                color: "#333",
              }}
            />
          )}
        />
        <FormHelperText id="SQLs-error">{errorMessage}</FormHelperText>
      </FormControl>
    </div>
  );
};
