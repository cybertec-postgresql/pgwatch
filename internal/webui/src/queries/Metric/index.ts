import { useMutation, useQuery } from "@tanstack/react-query";
import { QueryKeys } from "consts/queryKeys";
import { Metrics } from "types/Metric/Metric";
import { MetricRequestBody } from "types/Metric/MetricRequestBody";
import MetricService from "services/Metric";

const services = MetricService.getInstance();

export const useMetrics = () => useQuery<Metrics>({
  queryKey: [QueryKeys.Metric],
  queryFn: async () => await services.getMetrics()
});

export const useMetric = (name: string) => useQuery({
  queryKey: [QueryKeys.Metric, name],
  queryFn: async () => await services.getMetric(name),
  enabled: !!name
});

export const useDeleteMetric = () => useMutation({
  mutationKey: [QueryKeys.Metric],
  mutationFn: async (name: string) => await services.deleteMetric(name)
});

export const useEditMetric = () => useMutation({
  mutationKey: [QueryKeys.Metric],
  mutationFn: async (data: MetricRequestBody) => await services.editMetric(data),
});

export const useAddMetric = () => useMutation({
  mutationKey: [QueryKeys.Metric],
  mutationFn: async (data: MetricRequestBody) => await services.addMetric(data),
});
