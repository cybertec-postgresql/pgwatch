import { AutocompleteRenderInputParams, Autocomplete as MuiAutocomplete, TextField } from "@mui/material";
import { ControllerRenderProps } from "react-hook-form";

type Props = {
  id?: string;
  label: string;
  options: { label: string }[];
  loading?: boolean;
  error?: boolean;
} & ControllerRenderProps;

export const Autocomplete = (props: Props) => {
  const { id, label, options, loading, error, ...field } = props;

  const customInput = (params: AutocompleteRenderInputParams) => (
    <TextField
      {...params}
      label={label}
      error={error}
    />
  );

  return (
    <MuiAutocomplete
      {...field}
      id={id}
      options={options}
      renderInput={customInput}
      onChange={(_, value) => field.onChange(value ? value.label : "")}
      loading={loading}
      componentsProps={{
        popper: {
          modifiers: [
            {
              name: 'flip',
              enabled: false
            },
            {
              name: 'preventOverflow',
              enabled: false
            }
          ]
        }
      }}
    />
  );
};
