import { AlertColor } from "@mui/material";

import { useMutation } from "@tanstack/react-query";
import { queryClient } from "queryClient";

import { UseFormReset } from "react-hook-form";

import { QueryKeys } from "queries/queryKeys";
import { CreatePresetConfigForm, CreatePresetConfigRequestForm, UpdatePresetConfigRequestForm } from "queries/types/PresetTypes";

import PresetService from "services/Preset";

const services = PresetService.getInstance();

type mutationProps = {
  handleAlertOpen: (text: string, type: AlertColor) => void;
  handleClose: () => void;
  reset: UseFormReset<CreatePresetConfigForm>
}

export const useAddPreset = ({
  handleAlertOpen,
  handleClose,
  reset
}: mutationProps) => useMutation({
  mutationFn: async (data: CreatePresetConfigRequestForm) => await services.addPreset(data),
  onSuccess: (_data, variables) => {
    handleClose();
    queryClient.invalidateQueries({ queryKey: QueryKeys.preset });
    handleAlertOpen(`New preset config "${variables.pc_name}" has been successfully added!`, "success");
    reset();
  },
  onError: (error: any) => {
    handleAlertOpen(error.response.data, "error");
  }
});

export const useEditPreset = ({
  handleAlertOpen,
  handleClose,
  reset
}: mutationProps) => useMutation({
  mutationFn: async (data: UpdatePresetConfigRequestForm) => await services.editPreset(data),
  onSuccess: (_data, variables) => {
    handleClose();
    queryClient.invalidateQueries({ queryKey: QueryKeys.preset });
    handleAlertOpen(`Preset config ${variables.pc_name} has been successfully edited`, "success");
    reset();
  },
  onError: (error: any) => {
    handleAlertOpen(error.response.data, "error");
  }
});
