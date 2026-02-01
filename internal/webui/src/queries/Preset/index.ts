import { useMutation, useQuery } from "@tanstack/react-query";
import { QueryKeys } from "consts/queryKeys";
import { Presets } from "types/Preset/Preset";
import { PresetRequestBody } from "types/Preset/PresetRequestBody";
import PresetService from "services/Preset";

const services = PresetService.getInstance();

export const usePresets = () => useQuery<Presets>({
  queryKey: [QueryKeys.Preset],
  queryFn: async () => await services.getPresets()
});

export const usePreset = (name: string) => useQuery({
  queryKey: [QueryKeys.Preset, name],
  queryFn: async () => await services.getPreset(name),
  enabled: !!name
});

export const useDeletePreset = () => useMutation({
  mutationKey: [QueryKeys.Preset],
  mutationFn: async (name: string) => await services.deletePreset(name)
});

export const useEditPreset = () => useMutation({
  mutationKey: [QueryKeys.Preset],
  mutationFn: async (data: PresetRequestBody) => await services.editPreset(data),
});

export const useAddPreset = () => useMutation({
  mutationKey: [QueryKeys.Preset],
  mutationFn: async (data: PresetRequestBody) => await services.addPreset(data),
});

type ReorderPresetData = {
  Name: string;
  Data: {
    Description?: string;
    Metrics: Record<string, number>;
    SortOrder: number;
  };
};

export const useReorderPresets = () => useMutation({
  mutationKey: [QueryKeys.Preset],
  mutationFn: async (presets: ReorderPresetData[]) => {
    // Update all presets with their new sort orders
    await Promise.all(
      presets.map(preset => services.editPreset(preset))
    );
  },
});
