import { AlertColor } from "@mui/material";
import { useMutation, useQuery } from "@tanstack/react-query";
import { AxiosError } from "axios";
import { isUnauthorized } from "axiosInstance";
import { queryClient } from "queryClient";
import { UseFormReset } from "react-hook-form";
import { NavigateFunction } from "react-router-dom";
import { QueryKeys } from "queries/queryKeys";
import { Db, createDbForm, updateDbForm, updateEnabledDbForm } from "queries/types/DbTypes";
import DbService from "services/Db";

const services = DbService.getInstance();

export const useDbs = (callAlert: (severity: AlertColor, message: string) => void, navigate: NavigateFunction) => useQuery<Db[], AxiosError>({
  queryKey: QueryKeys.db,
  queryFn: async () => await services.getMonitoredDb(),
  onError: (error) => {
    if (isUnauthorized(error)) {
      callAlert("error", `${error.response?.data}`);
      navigate("/");
    }
  }
});

export const useDeleteDb = (callAlert: (severity: AlertColor, message: string) => void, navigate: NavigateFunction) => useMutation<any, AxiosError, string, unknown>({
  mutationFn: async (uniqueName: string) => await services.deleteMonitoredDb(uniqueName),
  onSuccess: (_data, variables) => {
    callAlert("success", `${variables} DB has been deleted`);
    queryClient.invalidateQueries({ queryKey: QueryKeys.db });
  },
  onError: (error) => {
    callAlert("error", `${error.response?.data}`);
    if (isUnauthorized(error)) {
      navigate("/");
    }
  }
});

export const useEditEnableDb = (callAlert: (severity: AlertColor, message: string) => void, navigate: NavigateFunction) => useMutation<any, AxiosError, updateEnabledDbForm, unknown>({
  mutationFn: async (data) => await services.editEnabledDb(data),
  onSuccess: (_data, variables) => {
    callAlert(
      "success",
      `${variables.md_unique_name} DB monitoring ${variables.data.md_is_enabled ? "enabled" : "disabled"}`
    );
    queryClient.invalidateQueries({ queryKey: QueryKeys.db });
  },
  onError: (error) => {
    callAlert("error", `${error.response?.data}`);
    if (isUnauthorized(error)) {
      navigate("/");
    }
  }
});

export const useEditDb = (
  callAlert: (severity: AlertColor, message: string) => void,
  navigate: NavigateFunction,
  handleClose: () => void,
  reset: UseFormReset<createDbForm>
) => useMutation<any, AxiosError, updateDbForm, unknown>({
  mutationFn: async (data) => await services.editMonitoredDb(data),
  onSuccess: (_data, variables) => {
    callAlert("success", `${variables.md_unique_name} DB updated`);
    handleClose();
    reset();
    queryClient.invalidateQueries({ queryKey: QueryKeys.db });
  },
  onError: (error) => {
    callAlert("error", `${error.response?.data}`);
    if (isUnauthorized(error)) {
      navigate("/");
    }
  }
});

export const useAddDb = (
  callAlert: (severity: AlertColor, message: string) => void,
  navigate: NavigateFunction,
  handleClose: () => void,
  reset: UseFormReset<createDbForm>
) => useMutation<any, AxiosError, createDbForm, unknown>({
  mutationFn: async (data) => await services.addMonitoredDb(data),
  onSuccess: (_data, variables) => {
    callAlert("success", `${variables.md_unique_name} DB added`);
    handleClose();
    reset();
    queryClient.invalidateQueries({ queryKey: QueryKeys.db });
  },
  onError: (error) => {
    callAlert("error", `${error.response?.data}`);
    if (isUnauthorized(error)) {
      navigate("/");
    }
  }
});
