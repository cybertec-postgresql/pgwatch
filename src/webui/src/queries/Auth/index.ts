import { AlertColor } from "@mui/material";
import { useMutation } from "@tanstack/react-query";
import { NavigateFunction } from "react-router-dom";
import { AuthForm } from "queries/types/AuthTypes";
import AuthService from "services/Auth";
import { removeToken, setToken } from "services/Token";

const services = AuthService.getInstance();

export const useLogin = (callAlert: (alertSeverity: AlertColor, alertMessage: string) => void, navigate: NavigateFunction) => useMutation({
  mutationFn: async(data: AuthForm) => await services.login(data),
  onSuccess: (data) => {
    setToken(data);
    callAlert("success", "Successfully logged in");
    navigate("/dashboard");
  },
  onError: () => {
    callAlert("error", "Can not authenticate user");
  }
});

export const logout = (navigate: NavigateFunction) => {
  removeToken();
  navigate("/");
};
