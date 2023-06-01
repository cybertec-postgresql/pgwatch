import { useQuery } from "@tanstack/react-query";
import { QueryKeys } from "queries/queryKeys";
import { StatsSummary } from "queries/types/StatsSummaryTypes";
import StatsSummaryService from "services/StatsSummary";


const services = StatsSummaryService.getInstance();

export const useStatsSummary = () => useQuery<StatsSummary>({
  queryKey: QueryKeys.statsSummary,
  queryFn: async () => await services.getStatsSummary()
});
