import { Autocomplete, AutocompleteRenderInputParams, TextField } from "@mui/material";
import { ControllerRenderProps } from "react-hook-form";

type AutocompleteProps = {
  field: ControllerRenderProps;
  label: string;
  options: { label: string }[];
  error: boolean;
  helperText?: string;
  loading?: boolean;
};

export const AutocompleteComponent = ({
  field: { value: initialValue, ...field },
  label,
  options,
  error,
  helperText,
  loading
}: AutocompleteProps) => {
  const customInput = (params: AutocompleteRenderInputParams) => (
    <TextField
      {...params}
      label={label}
      error={error}
      helperText={helperText}
    />
  );

  let value = options.find(option => option.label === String(initialValue));

  if (!value) {
    value = { label: initialValue };
  }

  return (
    <Autocomplete
      {...field}
      options={options}
      value={value}
      renderInput={customInput}
      onChange={(_, data) => field.onChange(data?.label ? data.label : "")}
      fullWidth
      loading={loading}
    />
  );
};
