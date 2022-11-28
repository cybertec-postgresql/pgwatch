import CopyIcon from "@mui/icons-material/CopyAllOutlined";
import VisibilityIcon from "@mui/icons-material/Visibility";
import {
  Autocomplete,
  AutocompleteRenderInputParams,
  Box,
  FormControl,
  IconButton,
  InputAdornment,
  InputLabel,
  MenuItem, Select, SelectChangeEvent, TextField, Tooltip
} from "@mui/material";

import { ControllerRenderProps, FieldPath } from "react-hook-form";

import { IFormInput } from "./ModalComponent";
import { dbTypeOptions, passwordEncryptionOptions, sslModeOptions } from "./SelectComponentsOptions";

type SelectParams = {
  field: ControllerRenderProps<IFormInput, FieldPath<IFormInput>>,
  label: string,
  title?: string,
  onChange?: ((event: SelectChangeEvent<string | number | boolean>, child: React.ReactNode) => void)
}

/*
    DbType select component
*/

export const DbTypeComponent = ({ field, label, title }: SelectParams) => {
  return (
    <FormControl fullWidth size="medium">
      <InputLabel>{label}</InputLabel>
      <Select
        {...field}
        label={label}
        title={title}
      >
        {dbTypeOptions.map((option) => {
          return (
            <MenuItem key={option.label} value={option.label}>{option.label}</MenuItem>
          );
        })}
      </Select>
    </FormControl>
  );
};

/*
    Password encryption select component
*/

export const PasswordEncryptionComponent = ({ field, label, title }: SelectParams) => {
  const options = passwordEncryptionOptions.map((option) => (
    <MenuItem key={option.label} value={option.label}>{option.label}</MenuItem>
  ));

  return (
    <Box>
      <FormControl fullWidth size="medium">
        <InputLabel>{label}</InputLabel>
        <Select
          {...field}
          label={label}
          title={title}
        >
          {options}
        </Select>
      </FormControl>
    </Box>
  );
};

/*
    SSL mode select component
*/

export const SslModeComponent = ({ field, label, title, onChange }: SelectParams) => {
  const opttions = sslModeOptions.map((option) => (
    <MenuItem key={option.value} value={option.value}>{option.label}</MenuItem>
  ));

  return (
    <Box>
      <FormControl fullWidth size="medium">
        <InputLabel>{label}</InputLabel>
        <Select
          {...field}
          label={label}
          title={title}
          onChange={onChange}
        >
          {opttions}
        </Select>
      </FormControl>
    </Box>
  );
};

/*
    Preset metrics config select component
*/

type SelectConfigParams = {
  field: ControllerRenderProps<IFormInput, FieldPath<IFormInput>>,
  label: string,
  options: { label: string }[];
  onCopyClick: () => void,
  onShowClick: () => void,
  helperText?: string;
}

export const AutocompleteConfigComponent = ({
  field: { value: initialValue, ...field },
  options,
  label,
  helperText,
  onCopyClick,
  onShowClick
}: SelectConfigParams) => {
  const startAdornment = (
    <>
      <Tooltip title="Show">
        <IconButton
          onClick={onShowClick}
          edge="start"
        >
          <VisibilityIcon />
        </IconButton>
      </Tooltip>
      <Tooltip title="Copy">
        <IconButton
          onClick={onCopyClick}
        >
          <CopyIcon />
        </IconButton>
      </Tooltip>
    </>
  );

  const customInput = (params: AutocompleteRenderInputParams) => (
    <TextField
      {...params}
      label={label}
      helperText={helperText}
      fullWidth
      InputProps={{
        ...params.InputProps,
        startAdornment: (
          <InputAdornment position="start">
            {startAdornment}
            {params.InputProps.startAdornment}
          </InputAdornment>
        ),
      }}
    />
  );

  const value = options.find(option => option.label === initialValue);

  return (
    <Autocomplete
      {...field}
      value={value}
      options={options}
      renderInput={customInput}
      onChange={(_, data) => field.onChange(data?.label ? data?.label : "")}
      sx={{
        width: "50%"
      }}
    />
  );
};
