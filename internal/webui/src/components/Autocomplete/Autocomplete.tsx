import { useState } from "react";
import {
  AutocompleteRenderInputParams,
  Autocomplete as MuiAutocomplete,
  Paper,
  Popper,
  TextField,
  Typography,
} from "@mui/material";
import { ControllerRenderProps } from "react-hook-form";

type Option = {
  label: string;
  description?: string;
};

type Props = {
  id?: string;
  label: string;
  options: Option[];
  loading?: boolean;
  error?: boolean;
} & ControllerRenderProps;

export const Autocomplete = (props: Props) => {
  const { id, label, options, loading, error, ...field } = props;
  const [hoveredOption, setHoveredOption] = useState<Option | null>(null);
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);

  const customInput = (params: AutocompleteRenderInputParams) => (
    <TextField {...params} label={label} error={error} />
  );

  const handleOptionMouseEnter = (
    option: Option,
    event: React.MouseEvent<HTMLElement>
  ) => {
    if (option.description) {
      setHoveredOption(option);
      setAnchorEl(event.currentTarget);
    }
  };

  const handleOptionMouseLeave = () => {
    setHoveredOption(null);
    setAnchorEl(null);
  };

  const renderOption = (
    liProps: React.HTMLAttributes<HTMLLIElement>,
    option: Option
  ) => (
    <li
      {...liProps}
      onMouseEnter={(e) => handleOptionMouseEnter(option, e)}
      onMouseLeave={handleOptionMouseLeave}
    >
      {option.label}
    </li>
  );

  return (
    <>
      <MuiAutocomplete
        {...field}
        id={id}
        options={options}
        renderInput={customInput}
        renderOption={renderOption}
        onChange={(_, value) => field.onChange(value ? value.label : "")}
        loading={loading}
        componentsProps={{
          popper: {
            modifiers: [
              {
                name: "flip",
                enabled: false,
              },
              {
                name: "preventOverflow",
                enabled: false,
              },
            ],
          },
        }}
      />
      <Popper
        open={Boolean(hoveredOption) && Boolean(anchorEl)}
        anchorEl={anchorEl}
        placement="right-start"
        style={{ zIndex: 1400 }}
      >
        <Paper elevation={4} sx={{ p: 2, maxWidth: 300 }}>
          <Typography variant="subtitle2" gutterBottom>
            {hoveredOption?.label}
          </Typography>
          <Typography variant="body2" color="text.secondary">
            {hoveredOption?.description}
          </Typography>
        </Paper>
      </Popper>
    </>
  );
};
