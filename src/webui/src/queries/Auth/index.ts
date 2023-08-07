import { AlertColor } from "@mui/material";
import { useMutation, useQuery } from "@tanstack/react-query";
import { NavigateFunction } from "react-router-dom";
import { AuthForm } from "queries/types/AuthTypes";
import AuthService from "services/Auth";
import { removeToken, setToken } from "services/Token";

const services = AuthService.getInstance();

export const useLogin = (navigate: NavigateFunction) => useMutation({
  mutationFn: async (data: AuthForm) => await services.login(data),
  onSuccess: (data) => {
    setToken(data);
    navigate("/dashboard");
  }
});

export const useLoginDefault = (navigate: NavigateFunction, callAlert: (alertSeverity: AlertColor, alertMessage: string) => void) => useQuery({
  queryFn: async () => {
    await services.loginDefault().
      then(data => {
        setToken(data);
        navigate("/dashboard");
      }).
      catch(() => callAlert("info", "Provide your credentials"));
  }
});

export const logout = (navigate: NavigateFunction) => {
  removeToken();
  navigate("/");
};
