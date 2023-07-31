import { useMutation, useQuery } from "@tanstack/react-query";
import { AxiosError } from "axios";

import { UseFormReset } from "react-hook-form";

import { QueryKeys } from "queries/queryKeys";
import { CreatePresetConfigForm, CreatePresetConfigRequestForm, Preset, UpdatePresetConfigRequestForm } from "queries/types/PresetTypes";

import PresetService from "services/Preset";

const services = PresetService.getInstance();

export const usePresets = () => useQuery<Preset[], AxiosError>({
  queryKey: QueryKeys.preset,
  queryFn: async () => await services.getPresets()
});

export const useDeletePreset = () => useMutation({
  mutationKey: QueryKeys.preset,
  mutationFn: async (data: string) => await services.deletePreset(data)
});

export const useEditPreset = (
  modalClose: () => void,
  reset: UseFormReset<CreatePresetConfigForm>
) => useMutation({
  mutationKey: QueryKeys.preset,
  mutationFn: async (data: UpdatePresetConfigRequestForm) => await services.editPreset(data),
  onSuccess: () => {
    modalClose();
    reset();
  }
});

export const useAddPreset = (
  modalClose: () => void,
  reset: UseFormReset<CreatePresetConfigForm>
) => useMutation({
  mutationKey: QueryKeys.preset,
  mutationFn: async (data: CreatePresetConfigRequestForm) => await services.addPreset(data),
  onSuccess: () => {
    modalClose();
    reset();
  }
});
