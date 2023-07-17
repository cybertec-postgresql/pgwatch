import { Box, Button, Stack, TextField, Typography } from "@mui/material";
import { Controller, SubmitHandler, useForm } from "react-hook-form";
import { useNavigate } from "react-router-dom";
import { AlertComponent } from "layout/common/AlertComponent";
import { useLogin } from "queries/Auth";
import { AuthForm } from "queries/types/AuthTypes";


export const SignIn = () => {
  const { handleSubmit, control } = useForm<AuthForm>();
  const login = useLogin();
  const navigate = useNavigate();

  const onSubmit: SubmitHandler<AuthForm> = (result) => {
    login.mutate(result);
  };

  if (login.isSuccess) {
    navigate("/dashboard", { replace: true });
  }

  return (
    <Box sx={{ flex: "1 1 auto", display: "flex", justifyContent: "center", alignItems: "center" }}>
      {
        login.isError && <AlertComponent severity="error" message="Can not authenticate this user" />
      }
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
              rules={{
                required: {
                  value: true,
                  message: "User name is required"
                }
              }}
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
              rules={{
                required: {
                  value: true,
                  message: "Password is required"
                }
              }}
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
