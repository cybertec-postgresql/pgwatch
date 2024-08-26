import { axiosInstance } from "axiosInstance";
import { LoginFormValues } from "pages/LoginPage/components/LoginForm/LoginForm.types";

export default class AuthService {
  private static _instance: AuthService;

  public static getInstance(): AuthService {
    if (!AuthService._instance) {
      AuthService._instance = new AuthService();
    }

    return AuthService._instance;
  };

  public async login(data: LoginFormValues) {
    return await axiosInstance.post("login", data).
      then(response => response.data);
  };
}
