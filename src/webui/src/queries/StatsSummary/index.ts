import { AlertColor } from "@mui/material";
import { useQuery } from "@tanstack/react-query";
import { AxiosError } from "axios";
import { isUnauthorized } from "axiosInstance";
import { NavigateFunction } from "react-router-dom";
import { QueryKeys } from "queries/queryKeys";
import { StatsSummary } from "queries/types/StatsSummaryTypes";
import StatsSummaryService from "services/StatsSummary";


const services = StatsSummaryService.getInstance();

export const useStatsSummary = (callAlert: (severity: AlertColor, message: string) => void, navigate: NavigateFunction) => useQuery<StatsSummary, AxiosError>({
  queryKey: QueryKeys.statsSummary,
  queryFn: async () => await services.getStatsSummary(),
  onError: (error) => {
    if (isUnauthorized(error)) {
      callAlert("error", `${error.response?.data}`);
      navigate("/");
    }
  }
});
