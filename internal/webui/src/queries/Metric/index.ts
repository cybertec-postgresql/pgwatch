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

export const useDeleteMetric = () => useMutation({
  mutationKey: [QueryKeys.Metric],
  mutationFn: async (data: string) => await services.deleteMetric(data)
});

export const useEditMetric = () => useMutation({
  mutationKey: [QueryKeys.Metric],
  mutationFn: async (data: MetricRequestBody) => await services.editMetric(data),
});

export const useAddMetric = () => useMutation({
  mutationKey: [QueryKeys.Metric],
  mutationFn: async (data: MetricRequestBody) => await services.addMetric(data),
});
