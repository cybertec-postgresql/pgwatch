import CopyIcon from "@mui/icons-material/CopyAllOutlined";
import VisibilityIcon from "@mui/icons-material/Visibility";
import {
  Autocomplete,
  AutocompleteRenderInputParams,
  IconButton,
  InputAdornment,
  TextField, Tooltip
} from "@mui/material";

import { ControllerRenderProps, FieldPath } from "react-hook-form";

import { IFormInput } from "./ModalComponent";

type SelectParams = {
  field: ControllerRenderProps<IFormInput, FieldPath<IFormInput>>,
  label: string,
  options: { label: string }[],
  error: boolean,
  helperText?: string,
  title: string,
};

export const AutocompleteComponent = ({
  field: { value: initialValue, ...field },
  label,
  options,
  error,
  helperText,
  title
}: SelectParams) => {
  const customInput = (params: AutocompleteRenderInputParams) => (
    <TextField
      {...params}
      label={label}
      error={error}
      helperText={helperText}
    />
  );

  const value = options.find(option => option.label === initialValue);

  return (
    <Autocomplete
      {...field}
      options={options}
      value={value}
      title={title}
      renderInput={customInput}
      onChange={(_, data) => field.onChange(data?.label ? data?.label : "")}
      fullWidth
    />
  );
};

type SelectSslModeParams = {
  field: ControllerRenderProps<IFormInput, FieldPath<IFormInput>>,
  label: string,
  options: { label: string }[],
  error: boolean,
  helperText?: string,
  title: string,
  handleChange: (nextValue?: string) => void
};

export const AutocompleteSslModeComponent = ({
  field: { value: initialValue, ...field },
  label,
  options,
  error,
  helperText,
  title,
  handleChange
}: SelectSslModeParams) => {
  const customInput = (params: AutocompleteRenderInputParams) => (
    <TextField
      {...params}
      label={label}
      error={error}
      helperText={helperText}
    />
  );

  const value = options.find(option => option.label === initialValue);

  return (
    <Autocomplete
      {...field}
      options={options}
      value={value}
      title={title}
      renderInput={customInput}
      onChange={(_, data) => {
        const nextValue = data?.label;
        field.onChange(nextValue ? nextValue : "");
        handleChange(nextValue);
      }}
      fullWidth
    />
  );
};

type SelectConfigParams = {
  field: ControllerRenderProps<IFormInput, FieldPath<IFormInput>>,
  label: string,
  options: { label: string }[],
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
