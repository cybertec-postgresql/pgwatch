import { useMutation, useQuery } from "@tanstack/react-query";
import { UseFormReset } from "react-hook-form";
import { QueryKeys } from "queries/queryKeys";
import { Db, createDbForm, updateDbForm, updateEnabledDbForm } from "queries/types/DbTypes";
import DbService from "services/Db";

const services = DbService.getInstance();

export const useDbs = () => useQuery<Db[]>({
  queryKey: QueryKeys.db,
  queryFn: async () => await services.getMonitoredDb()
});

export const useDeleteDb = () => useMutation({
  mutationKey: QueryKeys.db,
  mutationFn: async (uniqueName: string) => await services.deleteMonitoredDb(uniqueName)
});

export const useEditEnableDb = () => useMutation({
  mutationKey: QueryKeys.db,
  mutationFn: async (data: updateEnabledDbForm) => await services.editEnabledDb(data)
});

export const useEditDb = (
  handleClose: () => void,
  reset: UseFormReset<createDbForm>
) => useMutation({
  mutationKey: QueryKeys.db,
  mutationFn: async (data: updateDbForm) => await services.editMonitoredDb(data),
  onSuccess: () => {
    handleClose();
    reset();
  }
});

export const useAddDb = (
  handleClose: () => void,
  reset: UseFormReset<createDbForm>
) => useMutation({
  mutationKey: QueryKeys.db,
  mutationFn: async (data: createDbForm) => await services.addMonitoredDb(data),
  onSuccess: () => {
    handleClose();
    reset();
  }
});

export const useTestConnection = () => useMutation({
  mutationFn: async (data: string) => await services.testDbConnection(data)
});
