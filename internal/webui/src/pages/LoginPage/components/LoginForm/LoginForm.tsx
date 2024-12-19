import { yupResolver } from "@hookform/resolvers/yup";
import { Button, FormControl, FormHelperText, InputLabel, OutlinedInput, Typography } from "@mui/material";
import { PasswordInput } from "components/PasswordInput/PasswordInput";
import { SubmitHandler, useForm } from "react-hook-form";
import { useNavigate } from "react-router-dom";
import { useFormStyles } from "styles/form";
import { useLogin } from "queries/Auth";
import { loginFormValuesValidationSchema } from "./LoginForm.consts";
import { useLoginFormStyles } from "./LoginForm.styles";
import { LoginFormValues } from "./LoginForm.types";

export const LoginForm = () => {
  const { classes: loginFormClasses, cx } = useLoginFormStyles();
  const { classes: formClasses } = useFormStyles();

  const { register, handleSubmit, formState: { errors } } = useForm<LoginFormValues>({
    resolver: yupResolver(loginFormValuesValidationSchema),
  });

  const navigate = useNavigate();
  const login = useLogin(navigate);

  const getError = (field: keyof LoginFormValues) => {
    const error = errors[field];
    return error && error.message;
  };

  const onSubmit: SubmitHandler<LoginFormValues> = (values) => {
    login.mutate(values);
  };

  return (
    <>
      <Typography variant="h3">Login</Typography>
      <form onSubmit={handleSubmit(onSubmit)} className={loginFormClasses.form}>
        <FormControl
          className={cx(formClasses.formControlInput, formClasses.widthDefault)}
          error={!!getError("user")}
          variant="outlined"
        >
          <InputLabel htmlFor="user">User</InputLabel>
          <OutlinedInput
            {...register("user")}
            id="user"
            label="User"
          />
          <FormHelperText>{getError("user")}</FormHelperText>
        </FormControl>
        <FormControl
          className={cx(formClasses.formControlInput, formClasses.widthDefault)}
          error={!!getError("password")}
          variant="outlined"
        >
          <InputLabel htmlFor="password">Password</InputLabel>
          <PasswordInput
            {...register("password")}
            id="password"
            label="Password"
          />
          <FormHelperText>{getError("password")}</FormHelperText>
        </FormControl>
        <Button
          type="submit"
          variant="contained"
        >
          Login
        </Button>
      </form>
    </>
  );
};
