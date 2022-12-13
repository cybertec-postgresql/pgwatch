import { Box, Button, FormControlLabel, InputAdornment, MenuItem, Select, TextField, Typography } from "@mui/material";
import { Controller, SubmitHandler, useForm } from "react-hook-form";
import { filterEventOptions } from "./FilterEventOptions";

type IFormInput = {
  auto_refresh: number;
  event: string;
}

export const TopPanel = () => {
  const { control, handleSubmit } = useForm<IFormInput>({
    defaultValues: {
      auto_refresh: 20,
      event: "ALL"
    }
  });

  const onSubmit: SubmitHandler<IFormInput> = data => {
    alert(JSON.stringify(data));
  };

  return (
    <form onSubmit={handleSubmit(onSubmit)}>
      <Box sx={{ width: "100%", minHeight: 50, backgroundColor: "primary.dark", display: "flex", pt: 1, pb: 1, pl: 15, pr: 15 }}>
        <Box sx={{ display: "flex", flexGrow: 1 }}>
          <Controller
            name="auto_refresh"
            control={control}
            rules={{
              required: true
            }}
            render={({ field, fieldState: { error } }) => (
              <FormControlLabel
                disableTypography={false}
                componentsProps={{
                  typography: { variant: "h6" }
                }}
                label="Auto refresh"
                labelPlacement="start"
                sx={{ color: "white" }}
                control={
                  <TextField
                    {...field}
                    type="number"
                    error={!!error}
                    sx={{ input: { color: "white" }, width: 100, marginLeft: 1 }}
                    size="small"
                    InputProps={{
                      inputProps: {
                        min: 1, max: 60
                      },
                      startAdornment: <InputAdornment position="start">
                        <Typography sx={{ color: "#F5F3F3" }}>sec</Typography>
                      </InputAdornment>,
                    }}
                  />
                }
              />
            )}
          />
          <Controller
            name="event"
            control={control}
            render={({ field }) => (
              <FormControlLabel
                disableTypography={false}
                componentsProps={{
                  typography: { variant: "h6" }
                }}
                label="Filter by event"
                labelPlacement="start"
                sx={{ marginLeft: 10, color: "white" }}
                control={
                  <Select
                    {...field}
                    size="small"
                    sx={{ color: "white", width: 125, marginLeft: 1 }}
                  >
                    {filterEventOptions.map(event => (
                      <MenuItem key={event.label} value={event.label}>{event.label}</MenuItem>
                    ))}
                  </Select>
                }
              />
            )}
          />
        </Box>
        <Box sx={{ display: "flex" }}>
          <Button type="submit" sx={{ color: "#fff" }} variant="contained" size="large">Refresh</Button>
        </Box>
      </Box>
    </form>
  );
};
