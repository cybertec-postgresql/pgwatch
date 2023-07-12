import axios from "axios";
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
    return await axios.post("/login", data).
      then(response => response.data);
  };
}
