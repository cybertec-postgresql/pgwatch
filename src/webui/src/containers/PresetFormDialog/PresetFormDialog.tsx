import { useEffect, useMemo } from "react";
import { yupResolver } from "@hookform/resolvers/yup";
import { Button, Dialog, DialogActions, DialogContent } from "@mui/material";
import { usePresetFormContext } from "contexts/PresetForm/PresetForm.context";
import { FormProvider, SubmitHandler, useForm } from "react-hook-form";
import { useFormStyles } from "styles/form";
import { useAddPreset, useEditPreset } from "queries/Preset";
import { createPresetRequest, getPresetInitialValues } from "./PresetFormDialog.consts";
import { PresetForm } from "./components/PresetForm/PresetForm";
import { presetFormValuesValidationSchema } from "./components/PresetForm/PresetForm.consts";
import { PresetFormValues } from "./components/PresetForm/PresetForm.types";

export const PresetFormDialog = () => {
  const { data, open, handleClose } = usePresetFormContext();
  const formMethods = useForm<PresetFormValues>({
    resolver: yupResolver(presetFormValuesValidationSchema)
  });
  const { handleSubmit, reset } = formMethods;
  const { classes } = useFormStyles();

  useEffect(() => {
    const initialValues = getPresetInitialValues(data);
    reset(initialValues);
  }, [data, open, reset]);

  const addPreset = useAddPreset();
  const editPreset = useEditPreset();

  const isSuccess = useMemo(
    () => addPreset.isSuccess || editPreset.isSuccess,
    [addPreset.isSuccess, editPreset.isSuccess],
  );
  const isLoading = useMemo(
    () => addPreset.isLoading || editPreset.isLoading,
    [addPreset.isLoading, editPreset.isLoading]
  );

  useEffect(() => {
    if (isSuccess) {
      handleClose();
    }
  }, [isSuccess]); // eslint-disable-line

  const submitTitle = useMemo(
    () => data ? "Update preset" : "Add preset",
    [data],
  );

  const onSubmit: SubmitHandler<PresetFormValues> = (values) => {
    const preset = createPresetRequest(values);
    if (data) {
      editPreset.mutate(preset);
    } else {
      addPreset.mutate(preset);
    }
  };

  return (
    <Dialog
      open={open}
      onClose={handleClose}
      className={classes.formDialog}
    >
      <FormProvider {...formMethods}>
        <form onSubmit={handleSubmit(onSubmit)}>
          <DialogContent>
            <PresetForm />
          </DialogContent>
          <DialogActions className={classes.formButtons}>
            <Button
              onClick={handleClose}
              size="medium"
              variant="outlined"
              disabled={isLoading}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              size="medium"
              variant="contained"
              disabled={isLoading}
            >
              {submitTitle}
            </Button>
          </DialogActions>
        </form>
      </FormProvider>
    </Dialog>
  );
};
