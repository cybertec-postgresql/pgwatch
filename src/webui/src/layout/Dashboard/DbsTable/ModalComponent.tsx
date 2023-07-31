import { Dispatch, SetStateAction, useEffect, useState } from "react";
import CloseIcon from "@mui/icons-material/Close";
import DoneIcon from "@mui/icons-material/Done";
import Visibility from '@mui/icons-material/Visibility';
import VisibilityOff from '@mui/icons-material/VisibilityOff';
import {
  Box,
  Button,
  Checkbox,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  IconButton,
  InputAdornment,
  Stack,
  TextField,
} from "@mui/material";
import ToggleButton from "@mui/material/ToggleButton";
import ToggleButtonGroup from "@mui/material/ToggleButtonGroup";
import { Controller, FieldPath, FormProvider, SubmitHandler, useForm, useFormContext } from "react-hook-form";
import { useAddDb, useEditDb } from "queries/Dashboard";
import { Db, createDbForm } from "queries/types/DbTypes";
import {
  AutocompleteComponent,
  AutocompleteConfigComponent,
  AutocompleteSslModeComponent
} from "./SelectComponents";
import { dbTypeOptions, passwordEncryptionOptions, presetConfigsOptions, sslModeOptions } from "./SelectComponentsOptions";
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
        Object.entries(recordData!).map(([key, value]) => setValue(key as FieldPath<createDbForm>, key === "md_unique_name" ? "" : convertValue(value)));
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
        md_unique_name: recordData!.md_unique_name,
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
  ssl = "SSL",
  presets = "Presets"
}

type StepType = keyof typeof Steps;
const defaultStep = Object.keys(Steps)[0] as StepType;

const formErrors = {
  main: ["md_unique_name", "md_group", "md_dbtype", "md_password_type"],
  connection: ["md_hostname", "md_port", "md_user", "md_statement_timeout_seconds", "md_dbname"],
  ssl: ["md_sslmode"],
  presets: []
};

const getStepError = (step: StepType, errors: string[]): boolean => {
  const fields: string[] = formErrors[step];
  return errors.some(error => fields.includes(error));
};

const ModalContent = () => {
  const { control, formState: { errors }, getValues, setValue, watch } = useFormContext();
  const [activeStep, setActiveStep] = useState<StepType>(defaultStep);
  const [showPassword, setShowPassword] = useState<boolean>(false);

  const handleClickShowPassword = () => setShowPassword((show: boolean) => !show);

  const handleValidate = (val: string) => !!val.toString().trim();

  const testConnection = () => {
    const values = getValues();
    alert(`test connection...${JSON.stringify(values)}`);
  };

  const mdSslmodeVal = watch("md_sslmode");
  const isSslDisable = !mdSslmodeVal || mdSslmodeVal === "disable";

  const handleSslChange = (nextValue?: string) => {
    if (nextValue === "disable") {
      for (const inputName of ["md_root_ca_path", "md_client_cert_path", "md_client_key_path"]) {
        setValue(inputName, "");
      }
    }
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
            name="md_unique_name"
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
        <Controller
          name="md_password_type"
          control={control}
          defaultValue="plain-text"
          rules={{
            required: {
              value: true,
              message: "Password encryption is required"
            },
            validate: handleValidate
          }}
          render={({ field, fieldState: { error } }) => (
            <AutocompleteComponent
              field={{ ...field }}
              label="Password encryption"
              error={!!error}
              helperText={error?.message}
              options={passwordEncryptionOptions}
              title="The login role for actual metrics fetching from the specified host"
            />
          )}
        />
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
          <Controller
            name="md_is_superuser"
            control={control}
            defaultValue={false}
            render={({ field }) => (
              <FormControlLabel
                label="Super user?"
                labelPlacement="end"
                control={<Checkbox {...field} size="medium" checked={field.value} />}
              />
            )}
          />
        </Stack>
      </Stack>
    ),
    connection: (
      <Stack spacing={2}>
        <Stack direction="row" spacing={1}>
          <Controller
            name="md_hostname"
            control={control}
            defaultValue=""
            rules={{
              required: {
                value: true,
                message: "DB host is required"
              },
              validate: handleValidate,
            }}
            render={({ field, fieldState: { error } }) => (
              <SimpleTextField
                field={{ ...field }}
                error={!!error}
                helperText={error?.message}
                type="text"
                label="DB host"
              />
            )}
          />
          <Controller
            name="md_port"
            control={control}
            defaultValue={5432}
            rules={{
              required: {
                value: true,
                message: "DB port is required"
              },
              validate: handleValidate
            }}
            render={({ field, fieldState: { error } }) => (
              <SimpleTextField
                field={{ ...field }}
                error={!!error}
                helperText={error?.message}
                type="number"
                label="DB port"
                inputProps={{
                  inputProps: {
                    min: 1
                  }
                }}
              />
            )}
          />
        </Stack>
        <Controller
          name="md_dbname"
          control={control}
          defaultValue=""
          rules={{
            required: {
              value: true,
              message: "DB name is required"
            }
          }}
          render={({ field, fieldState: { error } }) => (
            <TextField
              {...field}
              error={!!error}
              helperText={error?.message}
              type="text"
              label="DB name"
              size="medium"
              fullWidth
              title="If left empty, all non-template DBs found from the cluster will be added to monitoring (if not already monitored), prefixed with the 'Unique name'. For 'pgbouncer' DB type insert the 'pool' name. Not relevant for 'postgres-continuous-discovery'"
            />
          )}
        />
        <Stack direction="row" spacing={1}>
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
        </Stack>
        <Stack direction="row" spacing={1}>
          <Controller
            name="md_user"
            control={control}
            defaultValue=""
            rules={{
              required: {
                value: true,
                message: "DB user is required"
              },
              validate: handleValidate
            }}
            render={({ field, fieldState: { error } }) => (
              <SimpleTextField
                field={{ ...field }}
                error={!!error}
                helperText={error?.message}
                type="text"
                label="DB user"
                title="The login role for actual metrics fetching from the specified host"
              />
            )}
          />
          <Controller
            name="md_password"
            control={control}
            defaultValue=""
            render={({ field }) => (
              <SimpleTextField
                field={{ ...field }}
                type={showPassword ? "text" : "password"}
                label="DB password"
                title="By default password is stored in plain-text in the database"
                inputProps={{
                  endAdornment: (
                    <InputAdornment position="end">
                      <IconButton
                        onClick={handleClickShowPassword}
                        edge="end"
                      >
                        {showPassword ? <VisibilityOff /> : <Visibility />}
                      </IconButton>
                    </InputAdornment>
                  )
                }}
              />
            )}
          />
        </Stack>
        <Stack direction="row" spacing={1}>
          <Controller
            name="md_statement_timeout_seconds"
            control={control}
            defaultValue={5}
            rules={{
              required: {
                value: true,
                message: "Statement timeout is required"
              },
              validate: handleValidate
            }}
            render={({ field, fieldState: { error } }) => (
              <SimpleTextField
                field={{ ...field }}
                error={!!error}
                helperText={error?.message}
                type="number"
                label="Statement timeout [seconds]"
                title="In seconds. Should stay low for critical DBs just in case. For possibly long-running built-in bloat estimation metrics the timeout will be max(30min,$userInput)"
                inputProps={{
                  inputProps: {
                    min: 1, max: 60
                  }
                }}
              />
            )}
          />
          <Controller
            name="connection_timeout_seconds"
            control={control}
            defaultValue=""
            render={({ field }) => (
              <SimpleTextField
                field={{ ...field }}
                type="number"
                label="Connection timeout [seconds]"
                inputProps={{
                  inputProps: {
                    min: 1, max: 60
                  }
                }}
              />
            )}
          />
        </Stack>
        <Button fullWidth variant="contained" onClick={testConnection}>Test connection</Button>
      </Stack>
    ),
    ssl: (
      <Stack spacing={2}>
        <Controller
          name="md_sslmode"
          control={control}
          defaultValue="disable"
          rules={{
            required: {
              value: true,
              message: "SSL mode is required"
            },
            validate: handleValidate
          }}
          render={({ field, fieldState: { error } }) => (
            <AutocompleteSslModeComponent
              field={{ ...field }}
              label="SSL mode"
              options={sslModeOptions}
              error={!!error}
              helperText={error?.message}
              title="libpq 'sslmode' parameter. If 'require' or 'verify-ca' or 'verify-full' then no metrics will be gathered if safe connection cannot be established"
              handleChange={handleSslChange}
            />
          )}
        />
        <Controller
          name="md_root_ca_path"
          control={control}
          defaultValue=""
          render={({ field }) => (
            <TextField
              {...field}
              type="text"
              label="Root CA"
              size="medium"
              disabled={isSslDisable}
              fullWidth
              title="Path to Root CA file on the gatherer. Relevant for sslmode-s 'verify-ca' and 'verify-full'"
            />
          )}
        />
        <Stack direction="row" spacing={1}>
          <Controller
            name="md_client_cert_path"
            control={control}
            defaultValue=""
            render={({ field }) => (
              <SimpleTextField
                field={{ ...field }}
                type="text"
                label="Client cert"
                disabled={isSslDisable}
                title="Path to Client certificate. Relevant for 'sslmode=verify-full'"
              />
            )}
          />
          <Controller
            name="md_client_key_path"
            control={control}
            defaultValue=""
            render={({ field }) => (
              <SimpleTextField
                field={{ ...field }}
                type="text"
                label="Client key"
                disabled={isSslDisable}
                title="Path to Client key file. Relevant for 'sslmode=verify-full'"
              />
            )}
          />
        </Stack>
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
            name="md_is_helpers"
            control={control}
            defaultValue={false}
            render={({ field }) => (
              <FormControlLabel
                label="Auto-create helpers?"
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
