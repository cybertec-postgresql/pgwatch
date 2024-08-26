import axios, { AxiosError } from "axios";
import { getToken } from "services/Token";

export const axiosInstance = axios.create({
  baseURL: "http://localhost:8080/"
});

axiosInstance.interceptors.request.use(req => {
  req.headers.set("Token", getToken());
  return req;
});

export const isUnauthorized = (error: AxiosError) => {
  if (error.response?.status === axios.HttpStatusCode.Unauthorized) {
    return true;
  }
  return false;
};
