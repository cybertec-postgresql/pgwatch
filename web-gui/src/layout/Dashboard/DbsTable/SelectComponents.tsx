import { Box, FormControl, InputLabel, Link, MenuItem, Select } from "@mui/material";

import { ControllerRenderProps, FieldPath } from "react-hook-form";

import { IFormInput } from "./ModalComponent";
import { dbTypeOptions, passwordEncryptionOptions, presetConfigsOptions, sslModeOptions } from "./SelectComponentsOptions";

type SelectParams = {
  field: ControllerRenderProps<IFormInput, FieldPath<IFormInput>>,
  label: string,
  title?: string
}

/*
    DbType select component
*/

export const DbTypeComponent = ({ field, label, title }: SelectParams) => {

  return (
    <Box sx={{ width: 236 }}>
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
    </Box>
  );
};

/*
    Password encryption select component
*/

export const PasswordEncryptionComponent = ({ field, label, title }: SelectParams) => {

  return (
    <Box sx={{ width: 236 }}>
      <FormControl fullWidth size="medium">
        <InputLabel>{label}</InputLabel>
        <Select
          {...field}
          label={label}
          title={title}
        >
          {passwordEncryptionOptions.map((option) => {
            return (
              <MenuItem key={option.label} value={option.label}>{option.label}</MenuItem>
            );
          })}
        </Select>
      </FormControl>
    </Box>
  );
};

/*
    SSL mode select component
*/

export const SslModeComponent = ({ field, label, title }: SelectParams) => {

  return (
    <Box sx={{ width: 236 }}>
      <FormControl fullWidth size="medium">
        <InputLabel>{label}</InputLabel>
        <Select
          {...field}
          label={label}
          title={title}
        >
          {sslModeOptions.map((option) => {
            return (
              <MenuItem key={option.label} value={option.label}>{option.label}</MenuItem>
            );
          })}
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
  title?: string,
  onCopyClick: () => void
}

export const MetricsConfigComponent = ({ field, label, title, onCopyClick }: SelectConfigParams) => {

  return (
    <Box sx={{ width: 236 }}>
      <FormControl fullWidth size="medium">
        <InputLabel>{label}</InputLabel>
        <Select
          {...field}
          label={label}
          title={title}
        >
          {presetConfigsOptions.map((option) => {
            return (
              <MenuItem key={option.label} value={option.label}>{option.label}</MenuItem>
            );
          })}
        </Select>
        <Box sx={{ display: "inline" }}>
          <Link href="#" underline="hover" sx={{ marginRight: "10px" }}>
            show
          </Link>
          <Link onClick={onCopyClick} href="javascript: void(0)" underline="hover">
            copy
          </Link>
        </Box>
      </FormControl>
    </Box>
  );
};

/*
    Standby preset config select component
*/

export const StandbyConfigComponent = ({ field, label, title, onCopyClick }: SelectConfigParams) => {

  return (
    <Box sx={{ width: 236 }}>
      <FormControl fullWidth size="medium">
        <InputLabel>{label}</InputLabel>
        <Select
          {...field}
          label={label}
          title={title}
        >
          {presetConfigsOptions.map((option) => {
            return (
              <MenuItem key={option.label} value={option.label}>{option.label}</MenuItem>
            );
          })}
        </Select>
        <Box sx={{ display: "inline" }}>
          <Link href="#" underline="hover" sx={{ marginRight: "10px" }}>
            show
          </Link>
          <Link onClick={onCopyClick} href="javascript: void(0)" underline="hover">
            copy
          </Link>
        </Box>
      </FormControl>
    </Box>
  );
};
