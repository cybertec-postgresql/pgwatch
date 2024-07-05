import { useEffect, useMemo } from "react";
import { yupResolver } from "@hookform/resolvers/yup";
import { Button, Dialog, DialogActions, DialogContent } from "@mui/material";
import { useSourceFormContext } from "contexts/SourceForm/SourceForm.context";
import { SourceFormActions } from "contexts/SourceForm/SourceForm.types";
import { FormProvider, SubmitHandler, useForm } from "react-hook-form";
import { useFormStyles } from "styles/form";
import { useAddSource, useEditSource } from "queries/Source";
import { createSourceRequest, getSourceInitialValues } from "./SourceFormDialog.consts";
import { SourceForm } from "./components/SourceForm/SourceForm";
import { sourceFormValuesValidationSchema } from "./components/SourceForm/SourceForm.consts";
import { SourceFormValues } from "./components/SourceForm/SourceForm.types";

export const SourceFormDialog = () => {
  const { data, open, action, handleClose } = useSourceFormContext();

  const formMethods = useForm<SourceFormValues>({
    resolver: yupResolver(sourceFormValuesValidationSchema),
  });
  const { handleSubmit, reset, setValue } = formMethods;

  const { classes } = useFormStyles();

  const addSource = useAddSource();
  const editSource = useEditSource();

  useEffect(() => {
    const initialValues = getSourceInitialValues(data);
    reset(initialValues);
    if (action === SourceFormActions.Copy) { setValue("DBUniqueName", ""); }
  }, [data, open, reset, action, setValue]);

  useEffect(() => {
    if (addSource.isSuccess || editSource.isSuccess) {
      handleClose();
    }
  }, [addSource.isSuccess, editSource.isSuccess, handleClose]);

  const submitTitle = useMemo(() => {
    if (action === SourceFormActions.Edit) {
      return "Update source";
    }
    return "Add source";
  }, [action]);

  const onSubmit: SubmitHandler<SourceFormValues> = (values) => {
    const source = createSourceRequest(values);
    if (action === SourceFormActions.Edit) {
      editSource.mutate(source);
    } else {
      addSource.mutate(source);
    }
  };

  const isLoading = useMemo(
    () => addSource.isLoading || editSource.isLoading,
    [addSource.isLoading, editSource.isLoading],
  );

  return (
    <Dialog open={open} onClose={handleClose} className={classes.formDialog}>
      <FormProvider {...formMethods}>
        <form onSubmit={handleSubmit(onSubmit)}>
          <DialogContent>
            <SourceForm />
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
