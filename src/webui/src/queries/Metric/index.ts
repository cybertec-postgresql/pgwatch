import { useMutation, useQuery } from "@tanstack/react-query";
import { UseFormReset } from "react-hook-form";
import { QueryKeys } from "queries/queryKeys";
import MetricService from "services/Metric";
import { createMetricForm, updateMetricForm } from "../types/MetricTypes";
import { Metrics } from "layout/MetricDefinitions/MetricDefinitions.types";

const services = MetricService.getInstance();

export const useMetrics = () => useQuery<Metrics>({
  queryKey: QueryKeys.metric,
  queryFn: async () => await services.getMetrics()
});

export const useDeleteMetric = () => useMutation({
  mutationKey: QueryKeys.metric,
  mutationFn: async (data: number) => await services.deleteMetric(data)
});

export const useEditMetric = (
  handleClose: () => void,
  reset: UseFormReset<createMetricForm>
) => useMutation({
  mutationKey: QueryKeys.metric,
  mutationFn: async (data: updateMetricForm) => await services.editMetric(data),
  onSuccess: () => {
    handleClose();
    reset();
  }
});

export const useAddMetric = (
  handleClose: () => void,
  reset: UseFormReset<createMetricForm>
) => useMutation({
  mutationKey: QueryKeys.metric,
  mutationFn: async (data: createMetricForm) => await services.addMetric(data),
  onSuccess: () => {
    handleClose();
    reset();
  }
});
