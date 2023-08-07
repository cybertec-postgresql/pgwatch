import { axiosInstance } from "axiosInstance";
import { AuthForm } from "queries/types/AuthTypes";

export default class AuthService {
  private static _instance: AuthService;

  public static getInstance(): AuthService {
    if (!AuthService._instance) {
      AuthService._instance = new AuthService();
    }

    return AuthService._instance;
  };

  public async login(data: AuthForm) {
    return await axiosInstance.post("login", data).
      then(response => response.data);
  };

  public async loginDefault() {
    return await axiosInstance.post("login", { user: "", password: "" }).
      then(response => response.data);
  };
}
