import { FilledInputProps, InputProps, OutlinedInputProps, TextField } from "@mui/material";
import { ControllerRenderProps, FieldPath } from "react-hook-form";
import { createDbForm } from "queries/types/DbTypes";


type Params = {
  field: ControllerRenderProps<createDbForm, FieldPath<createDbForm>>,
  error?: boolean,
  helperText?: string,
  type: "text" | "password" | "number",
  label: string,
  title?: string,
  disabled?: boolean,
  inputProps?: Partial<InputProps> | Partial<FilledInputProps> | Partial<OutlinedInputProps> | undefined
}

export const SimpleTextField = ({ field, error, helperText, type, label, title, disabled, inputProps }: Params) => {

  return (
    <TextField
      {...field}
      error={error}
      helperText={helperText}
      type={type}
      label={label}
      fullWidth
      title={title}
      disabled={disabled}
      InputProps={inputProps}
    />
  );
};

export const MultilineTextField = ({ field, error, helperText, type, label, title }: Params) => {

  return (
    <TextField
      {...field}
      error={error}
      helperText={helperText}
      type={type}
      label={label}
      multiline
      minRows={2}
      maxRows={5}
      title={title}
      sx={{
        flex: "1 1 50%"
      }}
    />
  );
};
