import { AlertColor } from "@mui/material";
import { useMutation, useQuery } from "@tanstack/react-query";
import { AxiosError } from "axios";
import { isUnauthorized } from "axiosInstance";
import { queryClient } from "queryClient";
import { UseFormReset } from "react-hook-form";
import { NavigateFunction } from "react-router-dom";
import { QueryKeys } from "queries/queryKeys";
import MetricService from "services/Metric";
import { Metric, createMetricForm, updateMetricForm } from "../types/MetricTypes";


const services = MetricService.getInstance();

export const useMetrics = (callAlert: (severity: AlertColor, message: string) => void, navigate: NavigateFunction) => useQuery<Metric[], AxiosError>({
  queryKey: QueryKeys.metric,
  queryFn: async () => await services.getMetrics(),
  onError: (error) => {
    if (isUnauthorized(error)) {
      callAlert("error", `${error.response?.data}`);
      navigate("/");
    }
  }
});

export const useUniqueMetrics = () => useQuery({
  queryKey: QueryKeys.metricUnique,
  queryFn: async () => {
    const data = await services.getMetrics();
    return ([...new Set(data.map(metric => metric.m_name))]);
  }
});

export const useDeleteMetric = (callAlert: (severity: AlertColor, message: string) => void, navigate: NavigateFunction) => useMutation<any, AxiosError, number, unknown>({
  mutationFn: async (data) => await services.deleteMetric(data),
  onSuccess: () => {
    callAlert("success", "Metric deleted");
    queryClient.invalidateQueries({ queryKey: QueryKeys.metric });
  },
  onError: (error) => {
    callAlert("error", `${error.response?.data}`);
    if (isUnauthorized(error)) {
      navigate("/");
    }
  }
});

export const useEditMetric = (
  callAlert: (severity: AlertColor, message: string) => void,
  navigate: NavigateFunction,
  handleClose: () => void,
  reset: UseFormReset<createMetricForm>
) => useMutation<any, AxiosError, updateMetricForm, unknown>({
  mutationFn: async (data) => await services.editMetric(data),
  onSuccess: (_data, variables) => {
    callAlert("success", `${variables.data.m_name} metric updated`);
    handleClose();
    reset();
    queryClient.invalidateQueries({ queryKey: QueryKeys.metric });
  },
  onError: (error) => {
    callAlert("error", `${error.response?.data}`);
    if (isUnauthorized(error)) {
      navigate("/");
    }
  }
});

export const useAddMetric = (
  callAlert: (severity: AlertColor, message: string) => void,
  navigate: NavigateFunction,
  handleClose: () => void,
  reset: UseFormReset<createMetricForm>
) => useMutation<any, AxiosError, createMetricForm, unknown>({
  mutationFn: async (data) => await services.addMetric(data),
  onSuccess: (_data, variables) => {
    callAlert("success", `${variables.m_name} metric added`);
    handleClose();
    reset();
    queryClient.invalidateQueries({ queryKey: QueryKeys.metric });
  },
  onError: (error) => {
    callAlert("error", `${error.response?.data}`);
    if (isUnauthorized(error)) {
      navigate("/");
    }
  }
});
