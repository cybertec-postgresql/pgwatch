import { useEffect, useState } from "react";
import TableViewIcon from "@mui/icons-material/TableView";
import { Button, Dialog, DialogActions, DialogContent, FormControl, FormHelperText, IconButton, InputLabel, OutlinedInput } from "@mui/material";
import { SubmitHandler, useForm } from "react-hook-form";
import { useFormStyles } from "styles/form";
import { Source } from "types/Source/Source";
import { useEditSourceHostConfig } from "queries/Source";
import { getHostConfigInitialValues } from "./HostConfigPopUp.consts";
import { HostConfigFormValues } from "./HostConfigPopUp.types";

type Props = {
  source: Source;
};

export const HostConfigPopUp = ({ source }: Props) => {
  const { classes, cx } = useFormStyles();

  const [dialogOpen, setDialogOpen] = useState(false);

  const { handleSubmit, reset, setError, register, formState: { errors, isDirty } } = useForm<HostConfigFormValues>();

  const { mutate, isSuccess } = useEditSourceHostConfig();

  useEffect(() => {
    const initialValues = getHostConfigInitialValues(source.HostConfig);
    reset(initialValues);
  }, [source.HostConfig, dialogOpen, reset]);

  useEffect(() => {
    if (isSuccess) {
      handleClose();
    }
  }, [isSuccess]); // eslint-disable-line

  const handleOpen = () => setDialogOpen(true);

  const handleClose = () => setDialogOpen(false);

  const hasError = (field: keyof HostConfigFormValues) => !!errors[field];

  const getError = (field: keyof HostConfigFormValues) => {
    const error = errors[field];
    return error ? error.message : undefined;
  };

  const onSubmit: SubmitHandler<HostConfigFormValues> = (values) => {
    try {
      const hostConfig = JSON.parse(values.HostConfig);
      mutate({
        ...source,
        HostConfig: hostConfig,
      });
    } catch (err) {
      setError("HostConfig", { message: "Invalid JSON" });
    }
  };

  return (
    <>
      <IconButton title="View host config" onClick={handleOpen}>
        <TableViewIcon />
      </IconButton>
      <Dialog
        open={dialogOpen}
        onClose={handleClose}
        maxWidth="md"
      >
        <form onSubmit={handleSubmit(onSubmit)}>
          <DialogContent sx={{ width: 450, maxHeight: 500 }}>
            <FormControl
              className={cx(classes.formControlInput, classes.widthFull)}
              error={hasError("HostConfig")}
              variant="outlined"
            >
              <InputLabel htmlFor="HostConfig">Host config</InputLabel>
              <OutlinedInput
                {...register("HostConfig")}
                id="HostConfig"
                label="Host Config"
                aria-describedby="HostConfig-error"
                type="text"
                multiline
                rows={15}
              />
              <FormHelperText id="HostConfig-error">{getError("HostConfig")}</FormHelperText>
            </FormControl>
          </DialogContent>
          <DialogActions className={classes.formButtons}>
            <Button
              onClick={handleClose}
              variant="outlined"
              size="medium"
            >
              Cancel
            </Button>
            <Button
              variant="contained"
              type="submit"
              size="medium"
              disabled={!isDirty}
            >
              Submit
            </Button>
          </DialogActions>
        </form>
      </Dialog>
    </>
  );
};
