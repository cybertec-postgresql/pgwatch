import DeleteIcon from "@mui/icons-material/Delete";
import { Button, FormControl, FormHelperText, IconButton, InputLabel, OutlinedInput } from "@mui/material";
import { useFieldArray, useFormContext } from "react-hook-form";
import { useFormStyles } from "styles/form";
import { SourceFormValues } from "../SourceForm.types";

export const SourceFormStepTags = () => {
  const { control, register, formState: { errors } } = useFormContext<SourceFormValues>();
  const { fields, append, remove } = useFieldArray({ control, name: "CustomTags" });

  const { classes, cx } = useFormStyles();

  const getError = (field: "Name" | "Value", index: number) => {
    const tagsErrors = errors.CustomTags;
    return tagsErrors && (field === "Name" ? tagsErrors[index]?.Name?.message : tagsErrors[index]?.Value?.message);
  };

  const handleAppend = () => {
    append({ Name: "", Value: "" });
  };

  return (
    <div className={classes.form}>
      {fields.map(({ id }, index) => (
        <div className={classes.row} key={id}>
          <FormControl
            className={cx(classes.formControlInput, classes.widthDefault)}
            error={!!getError("Name", index)}
            variant="outlined"
          >
            <InputLabel htmlFor={`CustomTags.${index}.Name`}>Tag Name</InputLabel>
            <OutlinedInput
              {...register(`CustomTags.${index}.Name`)}
              id={`CustomTags.${index}.Name`}
              label="Tag name"
            />
            <FormHelperText>{getError("Name", index)}</FormHelperText>
          </FormControl>
          <FormControl
            className={cx(classes.formControlInput, classes.widthDefault)}
            error={!!getError("Value", index)}
            variant="outlined"
          >
            <InputLabel htmlFor={`CustomTags.${index}.Value`}>Tag value</InputLabel>
            <OutlinedInput
              {...register(`CustomTags.${index}.Value`)}
              id={`CustomTags.${index}.Value`}
              label="Tag value"
              endAdornment={
                <IconButton
                  key={`CustomTags.${index}.Delete`}
                  title="Delete tag"
                  onClick={() => remove(index)}
                >
                  <DeleteIcon />
                </IconButton>
              }
            />
            <FormHelperText>{getError("Value", index)}</FormHelperText>
          </FormControl>
        </div>
      ))}
      <div className={cx(classes.row, classes.addButton)}>
        <Button
          variant="contained"
          onClick={handleAppend}
        >
          Add tag
        </Button>
      </div>
    </div>
  );
};
