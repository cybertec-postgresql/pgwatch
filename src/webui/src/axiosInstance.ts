import axios from "axios";
import { getToken } from "services/Token";

export const axiosInstance = axios.create({
  baseURL: "http://localhost:8080/"
});

axiosInstance.interceptors.request.use(req => {
  req.headers.set("Token", getToken());
  return req;
});
