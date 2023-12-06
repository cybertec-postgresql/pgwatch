import { Dispatch, SetStateAction, useEffect, useState } from "react";
import CloseIcon from "@mui/icons-material/Close";
import DoneIcon from "@mui/icons-material/Done";
import {
  Box,
  Button,
  Checkbox,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  Stack,
  TextField,
} from "@mui/material";
import ToggleButton from "@mui/material/ToggleButton";
import ToggleButtonGroup from "@mui/material/ToggleButtonGroup";
import { Controller, FieldPath, FormProvider, SubmitHandler, useForm, useFormContext } from "react-hook-form";
import { useAddDb, useEditDb, useTestConnection } from "queries/Dashboard";
import { Db, createDbForm } from "queries/types/DbTypes";
import {
  AutocompleteComponent,
  AutocompleteConfigComponent
} from "./SelectComponents";
import { dbTypeOptions, passwordEncryptionOptions, presetConfigsOptions } from "./SelectComponentsOptions";
import { MultilineTextField, SimpleTextField } from "./TextFieldComponents";

type Props = {
  open: boolean,
  setOpen: Dispatch<SetStateAction<boolean>>,
  recordData: Db | undefined,
  action: "NEW" | "EDIT" | "DUPLICATE"
};

export const ModalComponent = ({ open, setOpen, recordData, action }: Props) => {
  const methods = useForm<createDbForm>();
  const { handleSubmit, reset, setValue } = methods;

  useEffect(() => {
    switch (action) {
      case "NEW":
        reset();
        break;
      case "EDIT":
        reset();
        Object.entries(recordData!).map(([key, value]) => setValue(key as FieldPath<createDbForm>, convertValue(value)));
        break;
      case "DUPLICATE":
        reset();
        Object.entries(recordData!).map(([key, value]) => setValue(key as FieldPath<createDbForm>, key === "md_name" ? "" : convertValue(value)));
        break;
    }
  }, [action, recordData, reset, setValue]);

  const convertValue = (value: any): any => {
    if (typeof value === "object" && value !== null) {
      return JSON.stringify(value);
    } else {
      return value;
    }
  };

  const handleClose = () => setOpen(false);

  const editDb = useEditDb(handleClose, reset);

  const addDb = useAddDb(handleClose, reset);

  const onSubmit: SubmitHandler<createDbForm> = (result) => {
    if (action === "EDIT") {
      editDb.mutate({
        md_unique_name: recordData!.md_name,
        data: result
      });
    } else {
      addDb.mutate(result);
    }
  };

  return (
    <Dialog
      onClose={handleClose}
      open={open}
      fullWidth
      maxWidth="md"
    >
      <DialogTitle>{action === "EDIT" ? "Edit monitored database" : "Add new database to monitoring"}</DialogTitle>
      <FormProvider {...methods}>
        <form onSubmit={handleSubmit(onSubmit)}>
          <DialogContent>
            <ModalContent />
          </DialogContent>
          <DialogActions>
            <Button fullWidth onClick={handleClose} size="medium" variant="outlined" startIcon={<CloseIcon />}>Cancel</Button>
            <Button fullWidth type="submit" size="medium" variant="contained" startIcon={<DoneIcon />}>{action === "EDIT" ? "Submit changes" : "Start monitoring"}</Button>
          </DialogActions>
        </form>
      </FormProvider>
    </Dialog>
  );
};

enum Steps {
  main = "Main",
  connection = "Connection",
  presets = "Presets"
}

type StepType = keyof typeof Steps;
const defaultStep = Object.keys(Steps)[0] as StepType;

const formErrors = {
  main: ["md_name", "md_group", "md_dbtype"],
  connection: ["md_connstr", "md_encryption"],
  presets: []
};

const getStepError = (step: StepType, errors: string[]): boolean => {
  const fields: string[] = formErrors[step];
  return errors.some(error => fields.includes(error));
};

const ModalContent = () => {
  const { control, formState: { errors }, getValues, setValue } = useFormContext();
  const [activeStep, setActiveStep] = useState<StepType>(defaultStep);

  const testConnection = useTestConnection();

  const handleValidate = (val: string) => !!val.toString().trim();

  const handleTestConnection = () => {
    testConnection.mutate(getValues("md_connstr"));
  };

  const copyPresetConfig = (name: FieldPath<createDbForm>, value: string) => {
    setValue(name, value);
  };

  const showPresetConfig = (value: string) => {
    alert(`redirect to show preset config named: '${value}'`);
  };

  const stepContent = {
    main: (
      <Stack spacing={2}>
        <Stack direction="row" spacing={1}>
          <Controller
            name="md_name"
            control={control}
            rules={{
              required: {
                value: true,
                message: "Unique name is required"
              },
              validate: handleValidate
            }}
            defaultValue=""
            render={({ field, fieldState: { error } }) => (
              <SimpleTextField
                field={{ ...field }}
                error={!!error}
                helperText={error?.message}
                type="text"
                label="Unique name"
                title="Choose a good name as this shouldn't be changed later (cannot easily update InfluxDB data). Will be used as prefix during DB discovery mode"
              />
            )}
          />
          <Controller
            name="md_dbtype"
            control={control}
            defaultValue="postgres"
            rules={{
              required: {
                value: true,
                message: "DB type is required"
              },
              validate: handleValidate
            }}
            render={({ field, fieldState: { error } }) => (
              <AutocompleteComponent
                field={{ ...field }}
                label="DB type"
                error={!!error}
                helperText={error?.message}
                options={dbTypeOptions}
                title="For 'pgbouncer' insert the 'to be monitored' pool name to 'dbname' field or leave it empty to monitor all pools distinguished by the 'database' tag. For 'discovery' DB types one can also specify regex inclusion/exclusion patterns. For 'patroni' host/port are not used as it's read from DCS (specify DCS info under 'Host config')"
              />
            )}
          />
        </Stack>
        <Stack direction="row" spacing={1}>
          <Controller
            name="md_group"
            control={control}
            defaultValue="default"
            rules={{
              required: {
                value: true,
                message: "Group is required"
              },
              validate: handleValidate,
            }}
            render={({ field, fieldState: { error } }) => (
              <TextField
                {...field}
                error={!!error}
                helperText={error?.message}
                type="text"
                label="Group"
                fullWidth
                title="Group name (e.g. 'prod') for logical distinction of monitored databases. Can be used also to run multiple gatherers (sharding), one for each (or multiple) group(s). Required"
              />
            )}
          />
          <Controller
            name="md_custom_tags"
            control={control}
            defaultValue={null}
            render={({ field }) => (
              <TextField
                {...field}
                type="text"
                label="Custom tags"
                fullWidth
                title={"User defined tags for extra meaning in InfluxDB e.g. {\"env\": \"prod\", \"app\": \"xyz\"}"}
              />
            )}
          />
        </Stack>
        <Stack direction="row" spacing={1} sx={{ justifyContent: "center" }}>
          <Controller
            name="md_is_enabled"
            control={control}
            defaultValue={true}
            render={({ field }) => (
              <FormControlLabel
                label="Enabled?"
                labelPlacement="end"
                control={<Checkbox {...field} size="medium" checked={field.value} />}
                title="A tip - uncheck when leaving 'dbname' empty to review all found DBs before activation"
              />
            )}
          />
        </Stack>
      </Stack>
    ),
    connection: (
      <Stack spacing={2}>
        <Controller
          name="md_connstr"
          control={control}
          defaultValue=""
          rules={{
            required: {
              value: true,
              message: "Connection string is required"
            }
          }}
          render={({ field, fieldState: { error } }) => (
            <TextField
              {...field}
              type="text"
              label="Connection string"
              error={!!error}
              helperText={error?.message}
              multiline
              maxRows={3}
              fullWidth
            />
          )}
        />
        <Controller
          name="md_encryption"
          control={control}
          defaultValue="plain-text"
          rules={{
            required: {
              value: true,
              message: "Encryption is required"
            },
            validate: handleValidate
          }}
          render={({ field, fieldState: { error } }) => (
            <AutocompleteComponent
              field={{ ...field }}
              label="Encryption"
              error={!!error}
              helperText={error?.message}
              options={passwordEncryptionOptions}
              title="The login role for actual metrics fetching from the specified host"
            />
          )}
        />
        <Controller
          name="md_include_pattern"
          control={control}
          defaultValue={null}
          render={({ field }) => (
            <SimpleTextField
              field={{ ...field }}
              type="text"
              label="DB name inclusion pattern"
              title="POSIX regex input. Relevant only for 'discovery' DB types"
            />
          )}
        />
        <Controller
          name="md_exclude_pattern"
          control={control}
          defaultValue={null}
          render={({ field }) => (
            <SimpleTextField
              field={{ ...field }}
              type="text"
              label="DB name exclusion pattern"
              title="POSIX regex input. Relevant only for 'discovery' DB types"
            />
          )}
        />
        <Button fullWidth variant="contained" onClick={handleTestConnection}>
          {
            testConnection.isLoading ?
              (<CircularProgress size={25} sx={{ color: "white" }} />)
              :
              "Test connection"
          }
        </Button>
      </Stack>
    ),
    presets: (
      <Stack spacing={2} width="100%">
        <Stack direction="row" spacing={1}>
          <Controller
            name="md_preset_config_name"
            control={control}
            defaultValue="basic"
            render={({ field }) => (
              <AutocompleteConfigComponent
                field={{ ...field }}
                options={presetConfigsOptions}
                label="Preset metrics config"
                helperText="Preset config OR custom config needs to be specified"
                onCopyClick={() => copyPresetConfig("md_config", field.value)}
                onShowClick={() => showPresetConfig(field.value)}
              />
            )}
          />
          <Controller
            name="md_config"
            control={control}
            defaultValue={null}
            render={({ field }) => (
              <MultilineTextField
                field={{ ...field }}
                type="text"
                label="Custom metrics config"
                title="Optional alternate preset config for standby state to reduce the amount of gathered data for example"
              />
            )}
          />
        </Stack>
        <Stack direction="row" spacing={1}>
          <Controller
            name="md_preset_config_name_standby"
            control={control}
            defaultValue={null}
            render={({ field }) => (
              <AutocompleteConfigComponent
                field={{ ...field }}
                options={presetConfigsOptions}
                label="Standby preset config"
                helperText="Optional alternate preset config for standby state to reduce the amount of gathered data for example"
                onCopyClick={() => copyPresetConfig("md_config_standby", field.value)}
                onShowClick={() => showPresetConfig(field.value)}
              />
            )}
          />
          <Controller
            name="md_config_standby"
            control={control}
            defaultValue={null}
            render={({ field }) => (
              <MultilineTextField
                field={{ ...field }}
                type="text"
                label="Standby custom config"
                title={"{\"metric\":interval_seconds,...}"}
              />
            )}
          />
        </Stack>
        <Controller
          name="md_host_config"
          control={control}
          defaultValue={null}
          render={({ field }) => (
            <TextField
              {...field}
              type="text"
              label="Host config"
              multiline
              minRows={1}
              maxRows={5}
              title={"Used for Patroni only currently. e.g.: {\"dcs_type\": \"etcd|zookeeper|consul\", \"dcs_endpoints\": [\"http://127.0.0.1:2379/\"], \"namespace\": \"/service/\", \"scope\": \"batman\"}"}
            />
          )}
        />
        <Stack direction="row" spacing={1} sx={{ justifyContent: "center" }}>
          <Controller
            name="md_is_superuser"
            control={control}
            defaultValue={false}
            render={({ field }) => (
              <FormControlLabel
                label="Is superuser?"
                labelPlacement="end"
                control={<Checkbox {...field} size="medium" />}
                title="If checked the gatherer will automatically try to create the metric fetching helpers (CPU load monitoring, stat_statements, etc) - requires SUPERUSER to succeed"
              />
            )}
          />
          <Controller
            name="md_only_if_master"
            control={control}
            defaultValue={false}
            render={({ field }) => (
              <FormControlLabel
                label="Master mode only?"
                labelPlacement="end"
                control={<Checkbox {...field} size="medium" />}
                title="Fetch metrics only if master / primary"
              />
            )}
          />
        </Stack>
      </Stack>
    ),
  };

  const handleChange = (_ev: any, value?: StepType) => {
    if (value) {
      setActiveStep(value);
    }
  };

  const buttons = Object.entries(Steps).map(([val, label]) => (
    <ToggleButton
      fullWidth
      key={val}
      value={val}
      {...(getStepError(val as StepType, Object.keys(errors ?? {})) && {
        color: "error",
        selected: true,
      })}
    >
      {label}
    </ToggleButton>
  ));

  const content = Object.keys(Steps).map((key) => (
    <Box
      key={`Content-${key}`}
      {...(key !== activeStep && {
        height: 0,
        overflow: "hidden",
      })}
    >
      <>{stepContent[key as StepType]}</>
    </Box>
  ));

  return (
    <>
      <ToggleButtonGroup
        color="primary"
        value={activeStep}
        exclusive
        onChange={handleChange}
        aria-label="Platform"
        fullWidth
      >
        {buttons}
      </ToggleButtonGroup>
      <Box
        pt={2}
        // to prevent form content jumping
        minHeight="26vh"
      >
        <>{content}</>
      </Box>
    </>
  );
};
