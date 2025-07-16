import { useMutation, useQuery } from "@tanstack/react-query";
import { QueryKeys } from "consts/queryKeys";
import { Source } from "types/Source/Source";
import { SourceRequestBody } from "types/Source/SourceRequestBody";
import SourceService from "services/Source";

const services = SourceService.getInstance();

export const useSources = () => useQuery<Source[]>({
  queryKey: [QueryKeys.Source],
  queryFn: async () => await services.getSources()
});

export const useSource = (uniqueName: string) => useQuery({
  queryKey: [QueryKeys.Source, uniqueName],
  queryFn: async () => await services.getSource(uniqueName),
  enabled: !!uniqueName
});

export const useDeleteSource = () => useMutation({
  mutationKey: [QueryKeys.Source],
  mutationFn: async (uniqueName: string) => await services.deleteSource(uniqueName)
});

export const useEditSourceEnable = () => useMutation({
  mutationKey: [QueryKeys.Source],
  mutationFn: async (data: Source) => await services.editSourceEnable(data)
});

export const useEditSourceHostConfig = () => useMutation({
  mutationKey: [QueryKeys.Source],
  mutationFn: async (data: Source) => await services.editSourceHostConfig(data),
});

export const useEditSource = () => useMutation({
  mutationKey: [QueryKeys.Source],
  mutationFn: async (data: SourceRequestBody) => await services.editSource(data),
});

export const useAddSource = () => useMutation({
  mutationKey: [QueryKeys.Source],
  mutationFn: async (data: Source) => await services.addSource(data),
});

export const useTestConnection = () => useMutation({
  mutationFn: async (data: string) => await services.testSourceConnection(data)
});
