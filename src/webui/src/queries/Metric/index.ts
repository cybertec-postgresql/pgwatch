import { useQuery } from "@tanstack/react-query";
import { QueryKeys } from "queries/queryKeys";
import MetricService from "services/Metric";
import { Metric } from "../types/MetricTypes";


const services = MetricService.getInstance();

export const useMetrics = () => useQuery<Metric[]>({
  queryKey: QueryKeys.metric,
  queryFn: async () => await services.getMetrics()
});

export const useUniqueMetrics = () => useQuery({
  queryKey: QueryKeys.metric,
  queryFn: async () => {
    const data = await services.getMetrics();
    return ([...new Set(data.map(metric => metric.m_name))]);
  }
});
