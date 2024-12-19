import { useEffect, useMemo } from "react";
import { yupResolver } from "@hookform/resolvers/yup";
import { Button, Dialog, DialogActions, DialogContent } from "@mui/material";
import { useMetricFormContext } from "contexts/MetricForm/MetricForm.context";
import { FormProvider, SubmitHandler, useForm } from "react-hook-form";
import { useFormStyles } from "styles/form";
import { useAddMetric, useEditMetric } from "queries/Metric";
import { createMetricRequest, getMetricInitialValues } from "./MetricFormDialog.consts";
import { MetricForm } from "./components/MetricForm/MetricForm";
import { metricFormValuesValidationSchema } from "./components/MetricForm/MetricForm.consts";
import { MetricFormValues } from "./components/MetricForm/MetricForm.types";

export const MetricFormDialog = () => {
  const { data, open, handleClose } = useMetricFormContext();
  const formMethods = useForm<MetricFormValues>({
    resolver: yupResolver(metricFormValuesValidationSchema),
  });
  const { handleSubmit, reset, setError } = formMethods;
  const { classes } = useFormStyles();

  useEffect(() => {
    const initialValues = getMetricInitialValues(data);
    reset(initialValues);
  }, [data, open, reset]);

  const submitTitle = useMemo(
    () => data ? "Update metric" : "Add metric",
    [data]
  );

  const addMetric = useAddMetric();
  const editMetric = useEditMetric();

  const isSuccess = useMemo(
    () => addMetric.isSuccess || editMetric.isSuccess,
    [addMetric.isSuccess, editMetric.isSuccess],
  );
  const isLoading = useMemo(
    () => addMetric.isLoading || editMetric.isLoading,
    [addMetric.isLoading, editMetric.isLoading],
  );

  useEffect(() => {
    if (isSuccess) {
      handleClose();
    }
  }, [isSuccess]); // eslint-disable-line

  const onSubmit: SubmitHandler<MetricFormValues> = (values) => {
    try {
      const metric = createMetricRequest(values);
      if (data) {
        editMetric.mutate(metric);
      } else {
        addMetric.mutate(metric);
      }
    } catch (error: any) {
      setError("SQLs", { message: error.message });
    }
  };

  return (
    <Dialog open={open} onClose={handleClose} className={classes.formDialog}>
      <FormProvider {...formMethods}>
        <form onSubmit={handleSubmit(onSubmit)}>
          <DialogContent>
            <MetricForm />
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
