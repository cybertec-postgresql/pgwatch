import { useMutation, useQuery } from "@tanstack/react-query";
import { QueryKeys } from "consts/queryKeys";
import { UseFormReset } from "react-hook-form";
import { Source } from "types/Source/Source";
import { SourceEditEnabled } from "types/Source/SourceEditEnabled";
import SourceService from "services/Source";

const services = SourceService.getInstance();

export const useSources = () => useQuery<Source[]>({
  queryKey: [QueryKeys.Source],
  queryFn: async () => await services.getSources()
});

export const useDeleteSource = () => useMutation({
  mutationKey: [QueryKeys.Source],
  mutationFn: async (uniqueName: string) => await services.deleteSource(uniqueName)
});

export const useEditSourceEnable = () => useMutation({
  mutationKey: [QueryKeys.Source],
  mutationFn: async (data: SourceEditEnabled) => await services.editSourceEnable(data)
});

export const useEditSource = () => useMutation({
  mutationKey: [QueryKeys.Source],
  mutationFn: async (data) => await services.editSource(data),
});

export const useAddDb = () => useMutation({
  mutationKey: [QueryKeys.Source],
  mutationFn: async (data) => await services.addSource(data),
});

export const useTestConnection = () => useMutation({
  mutationFn: async (data: string) => await services.testSourceConnection(data)
});
