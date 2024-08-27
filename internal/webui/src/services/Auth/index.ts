import { apiClient } from "api";
import { AxiosInstance } from "axios";
import { LoginFormValues } from "pages/LoginPage/components/LoginForm/LoginForm.types";

export default class AuthService {
  private api: AxiosInstance;
  private static _instance: AuthService;

  constructor() {
    this.api = apiClient();
  }

  public static getInstance(): AuthService {
    if (!AuthService._instance) {
      AuthService._instance = new AuthService();
    }

    return AuthService._instance;
  };

  public async login(data: LoginFormValues) {
    return await this.api.post("/login", data).
      then(response => response.data);
  };
}
