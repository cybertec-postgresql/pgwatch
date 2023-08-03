import { Box, Button, Stack, TextField, Typography } from "@mui/material";
import { Controller, SubmitHandler, useForm } from "react-hook-form";
import { useNavigate } from "react-router-dom";
import { useLogin } from "queries/Auth";
import { AuthForm } from "queries/types/AuthTypes";


export const SignIn = () => {
  const { handleSubmit, control } = useForm<AuthForm>();
  const navigate = useNavigate();
  const login = useLogin(navigate);

  const onSubmit: SubmitHandler<AuthForm> = (result) => {
    login.mutate(result);
  };

  return (
    <Box sx={{ flex: "1 1 auto", display: "flex", justifyContent: "center", alignItems: "center" }}>
      <Box
        sx={{
          width: "25%",
          height: "auto",
          display: "flex",
          alignItems: "center",
          gap: 3,
          flexDirection: "column"
        }}
      >
        <Typography variant="h3" sx={{ fontWeight: "bold" }}>Login</Typography>
        <form onSubmit={handleSubmit(onSubmit)} style={{ width: "100%", gap: 24, display: "flex", flexFlow: "column" }}>
          <Stack spacing={1.5}>
            <Controller
              name="user"
              control={control}
              defaultValue=""
              render={({ field, fieldState: { error } }) => (
                <TextField
                  {...field}
                  error={!!error}
                  helperText={error?.message}
                  type="text"
                  label="User"
                  fullWidth
                />
              )}
            />
            <Controller
              name="password"
              control={control}
              defaultValue=""
              render={({ field, fieldState: { error } }) => (
                <TextField
                  {...field}
                  error={!!error}
                  helperText={error?.message}
                  type="password"
                  label="Password"
                  fullWidth
                />
              )}
            />
          </Stack>
          <Button fullWidth variant="contained" type="submit" >
            sign in
          </Button>
        </form>
      </Box>
    </Box>
  );
};
