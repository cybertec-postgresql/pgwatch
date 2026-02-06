import { FormControl, FormHelperText, InputLabel, OutlinedInput } from "@mui/material";
import { Controller, useFieldArray, useFormContext } from "react-hook-form";
import { useFormStyles } from "styles/form";
import { MetricFormValues } from "../MetricForm.types";
import { Button, FormControl, FormHelperText, IconButton, InputLabel, OutlinedInput } from "@mui/material";
import DeleteIcon from "@mui/icons-material/Delete";
import { Autocomplete } from "components/Autocomplete/Autocomplete";
import { useMemo } from "react";

export const MetricFormStepSQL = () => {
  const { control, register, formState: { errors } , clearErrors} = useFormContext<MetricFormValues>();
  const { classes, cx } = useFormStyles();
  const SqlFields = useFieldArray({ control, name: "SQLs" });

  const hasError = (field: keyof MetricFormValues) => !!errors[field];

  const getError = (field: keyof MetricFormValues) => {
    const error = errors[field];
    if (error) {
      return error.message;
    }
    return undefined;
  };
  const handleSqlAppend = () => {
    SqlFields.append({ Version: "", SQL: "" });
    clearErrors("PresetMetrics");
  };

  const VersionOptions = useMemo(() => [
      "15",
      "14",
      "13",
      "12",
      "11",
      "10",
    ].map((key) => ({ label: key })), 
    []
  );
  
  const getArrayError = (
  index: number,
  field: "Version" | "SQL"
  ) => errors.SQLs?.[index]?.[field]?.message;

  return (
    <div className={classes.form}>
        {SqlFields.fields.map(({ id }, index) => (
                  <div className={classes.row} key={id}>
                    <FormControl
                      className={cx(classes.formControlInput, classes.widthQuarter)}
                      variant="outlined"
                      error={!!getArrayError(index, "Version")}
                    >
                      <Controller
                        name={`SQLs.${index}.Version`}
                        control={control}
                        render={({ field }) => (
                          <Autocomplete
                            {...field}
                            id={`SQLs.${index}.Version`}
                            label="Version"
                            options={VersionOptions}
                            freeSolo
                            value={field.value}
                            onInputChange={(_, value) => field.onChange(value)}
                            // TDDO: Restrict input to numbers and dots only, but allow free solo for custom versions

                          />
                        )}
                      />
                      <FormHelperText>{getArrayError(index, "Version")}</FormHelperText>
                    </FormControl>
                    <FormControl
                      className={cx(classes.formControlInput, classes.widthThreeQuarter)}
                      variant="outlined"
                      error={hasError("SQLs")}
                    >
                      <InputLabel htmlFor={`SQLs.${index}.SQL`}>SQL</InputLabel>
                      <OutlinedInput
                        {...register(`SQLs.${index}.SQL`)}
                        id={`SQLs.${index}.SQL`}
                        label="SQL"
                        multiline
                        minRows={3}
                        maxRows={12}
                        endAdornment={
                          <IconButton
                            key={`SQLs.${index}.Delete`}
                            title="Delete SQL"
                            onClick={() => SqlFields.remove(index)}
                          >
                            <DeleteIcon />
                          </IconButton>
                        }
                      />
                      <FormHelperText>{getArrayError(index, "SQL")}</FormHelperText>
                    </FormControl>
                  </div>
                ))}
          <div className={cx(classes.row, classes.addButton)}>
          <Button
            variant="contained"
            onClick={handleSqlAppend}
          >
            Add SQLs
          </Button>
        </div>
        <FormHelperText id="SQLs-error">{getError("SQLs")}</FormHelperText>
    </div>
  );
};
