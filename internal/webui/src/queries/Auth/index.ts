import { useMutation } from "@tanstack/react-query";
import { LoginFormValues } from "pages/LoginPage/components/LoginForm/LoginForm.types";
import { NavigateFunction } from "react-router-dom";
import AuthService from "services/Auth";
import { removeToken, setToken } from "services/Token";

const services = AuthService.getInstance();

export const useLogin = (navigate: NavigateFunction) => useMutation({
  mutationFn: async (data: LoginFormValues) => await services.login(data),
  onSuccess: (data) => {
    setToken(data);
    navigate("/sources", { replace: true });
  }
});

export const logout = (navigate: NavigateFunction) => {
  removeToken();
  navigate("/", { replace: true });
};
