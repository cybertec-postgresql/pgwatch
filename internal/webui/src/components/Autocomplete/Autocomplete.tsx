import { AutocompleteRenderInputParams, Autocomplete as MuiAutocomplete, TextField, Box, Typography } from "@mui/material";
import { ControllerRenderProps } from "react-hook-form";

type AutocompleteOption = {
  label: string;
  description?: string;
};

type Props = {
  id?: string;
  label: string;
  options: AutocompleteOption[];
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
      getOptionLabel={(option) => typeof option === "string" ? option : option.label}
      renderOption={(props, option) => (
        <Box component="li" {...props}>
          <Box>
            <Typography variant="body1">{option.label}</Typography>
            {option.description && (
              <Typography variant="body2" color="text.secondary" sx={{ fontSize: '0.875rem' }}>
                {option.description}
              </Typography>
            )}
          </Box>
        </Box>
      )}
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
