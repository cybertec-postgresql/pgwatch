import axios, { AxiosError } from "axios";
import { getToken } from "services/Token";

export const apiClient = () => {
  const apiEndpoint = window.location.origin;
  const instance = axios.create({
    baseURL: apiEndpoint,
  });

  instance.interceptors.request.use(req => {
    req.headers.set("Token", getToken());
    return req;
  });

  return instance;
};

export const isUnauthorized = (error: AxiosError) => {
  if (error.response?.status === axios.HttpStatusCode.Unauthorized) {
    return true;
  }
  return false;
};
