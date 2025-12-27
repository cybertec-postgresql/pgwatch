import { useEffect } from "react";
import DeleteIcon from "@mui/icons-material/Delete";
import { Alert, Button, FormControl, FormHelperText, IconButton, InputLabel, OutlinedInput, Typography } from "@mui/material";
import { useFieldArray, useFormContext } from "react-hook-form";
import { useFormStyles } from "styles/form";
import { MetricFormValues } from "../MetricForm.types";

export const MetricFormStepSQL = () => {
  const { control, register, formState: { errors } } = useFormContext<MetricFormValues>();
  const { classes, cx } = useFormStyles();

  const { fields, append, remove } = useFieldArray({
    control,
    name: "SQLs"
  });

  // Initialize with one empty entry if no SQLs exist
  useEffect(() => {
    if (fields.length === 0) {
      append({ Version: 0, SQL: "" });
    }
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const handleAddSQL = () => {
    append({ Version: 0, SQL: "" });
  };

  const handleRemoveSQL = (index: number) => {
    if (fields.length > 1) {
      remove(index);
    }
  };

  const getFieldError = (index: number, field: "Version" | "SQL") => {
    const sqlsErrors = errors.SQLs;
    if (Array.isArray(sqlsErrors) && sqlsErrors[index]) {
      return sqlsErrors[index]?.[field]?.message;
    }
    return undefined;
  };

  const hasFieldError = (index: number, field: "Version" | "SQL") => {
    return !!getFieldError(index, field);
  };

  // Get the general error for the SQLs array (like duplicate versions)
  const getGeneralError = () => {
    if (errors.SQLs && !Array.isArray(errors.SQLs)) {
      return errors.SQLs.message;
    }
    return undefined;
  };

  return (
    <div className={classes.form}>
      <Typography variant="body2" sx={{ mb: 1, color: "text.secondary" }}>
        SQLs
      </Typography>

      {/* Show duplicate version error prominently */}
      {getGeneralError() && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {getGeneralError()}
        </Alert>
      )}

      {/* Scrollable SQL entries container */}
      <div style={{
        maxHeight: "400px",
        overflowY: "auto",
        overflowX: "hidden",
        marginBottom: "16px",
        paddingRight: "8px"
      }}>
        {fields.map((field, index) => (
          <div
            key={field.id}
            style={{
              border: "1px solid #e0e0e0",
              borderRadius: "4px",
              padding: "12px",
              marginBottom: "12px",
              backgroundColor: "#fafafa"
            }}
          >
            {/* SQL Entry Header */}
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "12px" }}>
              <Typography variant="body2" sx={{ fontWeight: 500, color: "text.secondary" }}>
                SQL #{index + 1}
              </Typography>
              <IconButton
                size="small"
                onClick={() => handleRemoveSQL(index)}
                disabled={fields.length === 1}
                sx={{ color: "error.main" }}
                title={fields.length === 1 ? "At least one SQL entry is required" : "Remove this SQL entry"}
              >
                <DeleteIcon fontSize="small" />
              </IconButton>
            </div>

            {/* Version and SQL fields in a row */}
            <div style={{ display: "grid", gridTemplateColumns: "120px 1fr", gap: "12px" }}>
              {/* Version Input */}
              <FormControl
                className={cx(classes.formControlInput)}
                error={hasFieldError(index, "Version")}
                variant="outlined"
                size="small"
              >
                <InputLabel htmlFor={`SQLs.${index}.Version`}>Version</InputLabel>
                <OutlinedInput
                  {...register(`SQLs.${index}.Version`, { valueAsNumber: true })}
                  id={`SQLs.${index}.Version`}
                  label="Version"
                  type="number"
                  inputProps={{ min: 1, step: 1 }}
                />
                {hasFieldError(index, "Version") && (
                  <FormHelperText>{getFieldError(index, "Version")}</FormHelperText>
                )}
              </FormControl>

              {/* SQL Textarea */}
              <FormControl
                className={cx(classes.formControlInput)}
                error={hasFieldError(index, "SQL")}
                variant="outlined"
                size="small"
              >
                <InputLabel htmlFor={`SQLs.${index}.SQL`}>SQL</InputLabel>
                <OutlinedInput
                  {...register(`SQLs.${index}.SQL`)}
                  id={`SQLs.${index}.SQL`}
                  label="SQL"
                  multiline
                  rows={3}
                  inputProps={{
                    style: {
                      font: "revert",
                      fontSize: "0.7rem",
                    }
                  }}
                />
                {hasFieldError(index, "SQL") && (
                  <FormHelperText>{getFieldError(index, "SQL")}</FormHelperText>
                )}
              </FormControl>
            </div>
          </div>
        ))}
      </div>

      {/* Add New SQL Button */}
      <Button
        variant="outlined"
        onClick={handleAddSQL}
        fullWidth
        sx={{ mb: 1 }}
      >
        + Add New SQL
      </Button>
    </div>
  );
};
