import axios from "axios";
import { AuthForm } from "queries/types/AuthTypes";
//import { getToken } from "services/Token";

/*const axiosInstance = axios.create({
  baseURL: "http://localhost:8080/",
  headers: {
    Authorization: getToken(),
    "Content-Type": "application/json",
    
  }
});*/

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
