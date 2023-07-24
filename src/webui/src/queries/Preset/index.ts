import { AlertColor } from "@mui/material";

import { useMutation, useQuery } from "@tanstack/react-query";
import { AxiosError } from "axios";
import { isUnauthorized } from "axiosInstance";
import { queryClient } from "queryClient";

import { UseFormReset } from "react-hook-form";

import { NavigateFunction } from "react-router-dom";
import { QueryKeys } from "queries/queryKeys";
import { CreatePresetConfigForm, CreatePresetConfigRequestForm, Preset, UpdatePresetConfigRequestForm } from "queries/types/PresetTypes";

import PresetService from "services/Preset";

const services = PresetService.getInstance();

export const usePresets = (callAlert: (severity: AlertColor, message: string) => void, navigate: NavigateFunction) => useQuery<Preset[], AxiosError>({
  queryKey: QueryKeys.preset,
  queryFn: async () => await services.getPresets(),
  onError: (error) => {
    if (isUnauthorized(error)) {
      callAlert("error", `${error.response?.data}`);
      navigate("/");
    }
  }
});

export const useDeletePreset = (callAlert: (severity: AlertColor, message: string) => void, navigate: NavigateFunction) => useMutation<any, AxiosError, string, unknown>({
  mutationFn: async (data) => await services.deletePreset(data),
  onSuccess: (_data, variables) => {
    callAlert("success", `${variables} preset deleted`);
    queryClient.invalidateQueries({ queryKey: QueryKeys.preset });
  },
  onError: (error) => {
    callAlert("error", `${error.response?.data}`);
    if (isUnauthorized(error)) {
      navigate("/");
    }
  }
});

export const useEditPreset = (
  callAlert: (severity: AlertColor, message: string) => void,
  navigate: NavigateFunction,
  modalClose: () => void,
  reset: UseFormReset<CreatePresetConfigForm>
) => useMutation<any, AxiosError, UpdatePresetConfigRequestForm, unknown>({
  mutationFn: async (data) => await services.editPreset(data),
  onSuccess: (_data, variables) => {
    callAlert("success", `${variables.pc_name} preset updated`);
    modalClose();
    reset();
    queryClient.invalidateQueries({ queryKey: QueryKeys.preset });
  },
  onError: (error) => {
    callAlert("error", `${error.response?.data}`);
    if (isUnauthorized(error)) {
      navigate("/");
    }
  }
});

export const useAddPreset = (
  callAlert: (severity: AlertColor, message: string) => void,
  navigate: NavigateFunction,
  modalClose: () => void,
  reset: UseFormReset<CreatePresetConfigForm>
) => useMutation<any, AxiosError, CreatePresetConfigRequestForm, unknown>({
  mutationFn: async (data) => await services.addPreset(data),
  onSuccess: (_data, variables) => {
    callAlert("success", `${variables.pc_name} preset added`);
    modalClose();
    reset();
    queryClient.invalidateQueries({ queryKey: QueryKeys.preset });
  },
  onError: (error) => {
    callAlert("error", `${error.response?.data}`);
    if (isUnauthorized(error)) {
      navigate("/");
    }
  }
});
