import axios, { AxiosError } from "axios";
import { getToken } from "services/Token";

export const apiClient = () => {
  // Use base path from window object if available
  const basePath = (window as any).__PGWATCH_BASE_PATH__ || '';
  const apiEndpoint = window.location.origin + basePath;
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
