import { useMutation } from "@tanstack/react-query";
import { AuthForm } from "queries/types/AuthTypes";
import AuthService from "services/Auth";
import { setToken } from "services/Token";

const services = AuthService.getInstance();

export const useLogin = () => useMutation({
  mutationFn: async(data: AuthForm) => await services.login(data),
  onSuccess: (data) => {
    setToken(data.accessToken);
  }
});
