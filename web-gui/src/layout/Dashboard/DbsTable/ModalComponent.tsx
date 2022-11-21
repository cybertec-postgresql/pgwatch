import { useEffect, useState } from "react";
import { Controller, FormProvider, SubmitHandler, useForm, useFormContext } from "react-hook-form";

import CloseIcon from "@mui/icons-material/Close";
import DoneIcon from "@mui/icons-material/Done";

import {
  Box,
  Button,
  Checkbox,
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


import {
  DbTypeComponent, MetricsConfigComponent, PasswordEncryptionComponent,
  SslModeComponent, StandbyConfigComponent
} from "./SelectComponents";
import { MultilineTextField, SimpleTextField } from "./TextFieldComponents";

type Props = {
  open: boolean,
  setOpen: React.Dispatch<React.SetStateAction<boolean>>,
  handleAlertOpen: (isOpen: boolean, text: string) => void,
  data: any
}

export interface IFormInput {
  md_unique_name: string;
  md_dbtype: string;
  md_hostname: string;
  md_port: number;
  md_dbname: string;
  md_include_pattern: string;
  md_exclude_pattern: string;
  md_user: string;
  md_password: string;
  md_password_type: string;
  md_sslmode: string;
  md_root_ca_path: string;
  md_client_cert_path: string;
  md_client_key_path: string;
  md_group: string;
  md_preset_config_name: string;
  md_config: string;
  md_preset_config_name_standby: string;
  md_config_standby: string;
  md_host_config: string;
  md_custom_tags: string;
  md_statement_timeout_seconds: number;
  helpers: boolean;
  md_only_if_master: boolean;
  md_is_enabled: boolean;
  connection_timeout_seconds: number;
}

export const ModalComponent = ({ open, setOpen, handleAlertOpen, data }: Props) => {
  const methods = useForm<IFormInput>();
  const { handleSubmit } = methods;

  const onSubmit: SubmitHandler<IFormInput> = result => {
    if (data) {
      data = { ...data, ...result };
      // Edit an already existing record
      alert(JSON.stringify(data));
      handleAlertOpen(true, "Success!");
    } else {
      // Create new record
      alert(JSON.stringify(result));
      handleAlertOpen(true, "Success!");
    }
  };

  const handleClose = () => setOpen(false);

  return (
    <Dialog onClose={handleClose} open={open}>
      <DialogTitle>{data ? "Edit database instance" : "Create new database instance"}</DialogTitle>
      <FormProvider {...methods}>
        <form onSubmit={handleSubmit(onSubmit)}>
          <DialogContent>
            <ModalContent />
          </DialogContent>
          <DialogActions>
            <Button fullWidth onClick={handleClose} size="medium" variant="outlined" startIcon={<CloseIcon />}>Cancel</Button>
            <Button fullWidth type="submit" size="medium" variant="contained" startIcon={<DoneIcon />}>Submit</Button>
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
  main: ["md_unique_name", "md_group"],
  connection: ["md_hostname", "md_port", "md_user", "md_statement_timeout_seconds"],
  // TODO: add other required fields
  ssl: [],
  presets: [],
}

const getStepError = (step: StepType, errors: string[]): boolean => {
  const fields: string[] = formErrors[step];
  return errors.some(error => fields.includes(error));
}

const ModalContent = () => {
  const { control, formState: { errors } } = useFormContext();
  const [activeStep, setActiveStep] = useState<StepType>(defaultStep);

  const handleValidate = (val: string) => !!val.toString().trim();

  const testConnection = () => {
    console.log("test connection...")
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
                title="NB! Choose a good name as this shouldn't be changed later (cannot easily update InfluxDB data). Will be used as prefix during DB discovery mode"
              />
            )}
          />
          <Controller
            name="md_dbtype"
            control={control}
            defaultValue="postgres"
            render={({ field }) => (
              <DbTypeComponent
                field={{ ...field }}
                label="DB type"
                title="NB! For 'pgbouncer' insert the 'to be monitored' pool name to 'dbname' field or leave it empty to monitor all pools distinguished by the 'database' tag. For 'discovery' DB types one can also specify regex inclusion/exclusion patterns. For 'patroni' host/port are not used as it's read from DCS (specify DCS info under 'Host config')"
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
          defaultValue=""
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
          name="md_is_enabled"
          control={control}
          defaultValue={true}
          render={({ field }) => (
            <FormControlLabel
              label="Enabled?"
              labelPlacement="end"
              control={<Checkbox {...field} size="medium" checked={field.value} />}
              sx={{ margin: 0 }}
              title="A tip - uncheck when leaving 'dbname' empty to review all found DBs before activation"
            />
          )}
        />
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
              />
            )}
          />
        </Stack>
        <Controller
          name="md_dbname"
          control={control}
          defaultValue=""
          render={({ field }) => (
            <TextField
              {...field}
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
            defaultValue=""
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
            defaultValue=""
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
                type="password"
                label="DB password"
                title="NB! By default password is stored in plain-text in the database"
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
                label="Statement timeount [seconds]"
                title="In seconds. Should stay low for critical DBs just in case. NB! For possibly long-running built-in bloat estimation metrics the timeout will be max(30min,$userInput)"
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
                label="Connection timeount [seconds]"
              />
            )}
          />
        </Stack>
        <Button fullWidth variant="contained" onClick={testConnection}>Test connection</Button>
      </Stack>
    ),
    ssl: (
      <Stack /> // TODO: add inputs
    ),
    presets: (
      <Stack /> // TODO: add inputs
    ),
  }

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
      <Box pt={2}>
        <>{content}</>
      </Box>






      {/*<Box sx={{ display: "flex", justifyContent: "space-between" }}>
      <Controller
        name="md_unique_name"
        control={control}
        rules={{
          required: "Unique name cannot be empty"
        }}
        defaultValue=""
        render={({ field }) => (
          <SimpleTextField
            field={{ ...field }}
            error={!!errors.md_unique_name}
            helperText={errors.md_unique_name?.message}
            type="text"
            label="Unique name"
            title="NB! Choose a good name as this shouldn't be changed later (cannot easily update InfluxDB data). Will be used as prefix during DB discovery mode"
          />
        )}
      />
      <Controller
        name="md_dbtype"
        control={control}
        defaultValue="postgres"
        render={({ field }) => (
          <DbTypeComponent
            field={{ ...field }}
            label="DB type"
            title="NB! For 'pgbouncer' insert the 'to be monitored' pool name to 'dbname' field or leave it empty to monitor all pools distinguished by the 'database' tag. For 'discovery' DB types one can also specify regex inclusion/exclusion patterns. For 'patroni' host/port are not used as it's read from DCS (specify DCS info under 'Host config')"
          />
        )}
      />
    </Box>
    <Box sx={{ display: "flex", justifyContent: "space-between" }}>
      <Controller
        name="md_hostname"
        control={control}
        rules={{
          required: "DB host cannot be empty"
        }}
        defaultValue=""
        render={({ field }) => (
          <SimpleTextField
            field={{ ...field }}
            error={!!errors.md_hostname}
            helperText={errors.md_hostname?.message}
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
          required: "DB port cannot be empty"
        }}
        render={({ field }) => (
          <SimpleTextField
            field={{ ...field }}
            error={!!errors.md_port}
            helperText={errors.md_port?.message}
            type="number"
            label="DB port"
          />
        )}
      />
    </Box>
    <Controller
      name="md_dbname"
      control={control}
      defaultValue=""
      render={({ field }) => (
        <TextField
          {...field}
          type="text"
          label="DB name"
          size="medium"
          fullWidth
          title="If left empty, all non-template DBs found from the cluster will be added to monitoring (if not already monitored), prefixed with the 'Unique name'. For 'pgbouncer' DB type insert the 'pool' name. Not relevant for 'postgres-continuous-discovery'"
        />
      )}
    />
    <Box sx={{ display: "flex", justifyContent: "space-between" }}>
      <Controller
        name="md_include_pattern"
        control={control}
        defaultValue=""
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
        defaultValue=""
        render={({ field }) => (
          <SimpleTextField
            field={{ ...field }}
            type="text"
            label="DB name exclusion pattern"
            title="POSIX regex input. Relevant only for 'discovery' DB types"
          />
        )}
      />
    </Box>
    <Box sx={{ display: "flex", justifyContent: "space-between" }}>
      <Controller
        name="md_user"
        control={control}
        defaultValue=""
        rules={{
          required: "DB user cannot be empty"
        }}
        render={({ field }) => (
          <SimpleTextField
            field={{ ...field }}
            error={!!errors.md_user}
            helperText={errors.md_user?.message}
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
            type="password"
            label="DB password"
            title="NB! By default password is stored in plain-text in the database"
          />
        )}
      />
    </Box>
    <Box sx={{ display: "flex", justifyContent: "space-between" }}>
      <Controller
        name="md_password_type"
        control={control}
        defaultValue="plain-text"
        render={({ field }) => (
          <PasswordEncryptionComponent
            field={{ ...field }}
            label="Password encryption"
            title="The login role for actual metrics fetching from the specified host"
          />
        )}
      />
      <Controller
        name="md_sslmode"
        control={control}
        defaultValue="disable"
        render={({ field }) => (
          <SslModeComponent
            field={{ ...field }}
            label="SSL Mode"
            title="libpq 'sslmode' parameter. If 'require' or 'verify-ca' or 'verify-full' then no metrics will be gathered if safe connection cannot be established"
          />
        )}
      />
    </Box>
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
          fullWidth
          title="Path to Root CA file on the gatherer. Relevant for sslmode-s 'verify-ca' and 'verify-full'"
        />
      )}
    />
    <Box sx={{ display: "flex", justifyContent: "space-between" }}>
      <Controller
        name="md_client_cert_path"
        control={control}
        defaultValue=""
        render={({ field }) => (
          <SimpleTextField
            field={{ ...field }}
            type="text"
            label="Client cert"
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
            title="Path to Client key file. Relevant for 'sslmode=verify-full'"
          />
        )}
      />
    </Box>
    <Controller
      name="md_group"
      control={control}
      defaultValue="default"
      rules={{
        required: "Group cannot be empty"
      }}
      render={({ field }) => (
        <TextField
          {...field}
          error={!!errors.md_group}
          helperText={errors.md_group?.message}
          type="text"
          label="Group"
          size="medium"
          fullWidth
          title="Group name (e.g. 'prod') for logical distinction of monitored databases. Can be used also to run multiple gatherers (sharding), one for each (or multiple) group(s). Required"
        />
      )}
    />
    <Box sx={{ display: "flex", justifyContent: "space-between" }}>
      <Controller
        name="md_preset_config_name"
        control={control}
        defaultValue=""
        render={({ field }) => (
          <MetricsConfigComponent
            field={{ ...field }}
            label="Preset metrics config"
            title="Preset config OR custom config needs to be specified"
            onCopyClick={() => setValue("md_config", field.value)}
          />
        )}
      />
      <Controller
        name="md_config"
        control={control}
        defaultValue=""
        render={({ field }) => (
          <MultilineTextField
            field={{ ...field }}
            type="text"
            label="Custom metrics config"
            title="Optional alternate preset config for standby state to reduce the amount of gathered data for example"
          />
        )}
      />
    </Box>
    <Box sx={{ display: "flex", justifyContent: "space-between" }}>
      <Controller
        name="md_preset_config_name_standby"
        control={control}
        defaultValue=""
        render={({ field }) => (
          <StandbyConfigComponent
            field={{ ...field }}
            label="Standby preset config"
            title="Optional alternate preset config for standby state to reduce the amount of gathered data for example"
            onCopyClick={() => setValue("md_config_standby", field.value)}
          />
        )}
      />
      <Controller
        name="md_config_standby"
        control={control}
        defaultValue=""
        render={({ field }) => (
          <MultilineTextField
            field={{ ...field }}
            type="text"
            label="Standby custom config"
            title={"{\"metric\":interval_seconds,...}"}
          />
        )}
      />
    </Box>
    <Box sx={{ display: "flex", justifyContent: "space-between" }}>
      <Controller
        name="md_host_config"
        control={control}
        defaultValue=""
        rules={{
          required: "Host config cannot be empty"
        }}
        render={({ field }) => (
          <TextField
            {...field}
            error={!!errors.md_host_config}
            helperText={errors.md_host_config?.message}
            type="text"
            label="Host config"
            multiline
            minRows={2}
            maxRows={5}
            sx={{ width: 236 }}
            title={"Used for Patroni only currently. e.g.: {\"dcs_type\": \"etcd|zookeeper|consul\", \"dcs_endpoints\": [\"http://127.0.0.1:2379/\"], \"namespace\": \"/service/\", \"scope\": \"batman\"}"}
          />
        )}
      />
      <Controller
        name="md_custom_tags"
        control={control}
        defaultValue=""
        rules={{
          required: "Custom tags cannot be empty"
        }}
        render={({ field }) => (
          <TextField
            {...field}
            error={!!errors.md_custom_tags}
            helperText={errors.md_custom_tags?.message}
            type="text"
            label="Custom tags"
            multiline
            minRows={2}
            maxRows={5}
            sx={{ width: 236 }}
            title={"User defined tags for extra meaning in InfluxDB e.g. {\"env\": \"prod\", \"app\": \"xyz\"}"}
          />
        )}
      />
    </Box>
    <Controller
      name="md_statement_timeout_seconds"
      control={control}
      defaultValue={5}
      rules={{
        required: "Statement timeout cannot be empty"
      }}
      render={({ field }) => (
        <TextField
          {...field}
          error={!!errors.md_statement_timeout_seconds}
          helperText={errors.md_statement_timeout_seconds?.message}
          type="number"
          label="Statement timeount [seconds]"
          title="In seconds. Should stay low for critical DBs just in case. NB! For possibly long-running built-in bloat estimation metrics the timeout will be max(30min,$userInput)"
        />
      )}
    />
    <Box sx={{ display: "flex", justifyContent: "space-between" }}>
      <Controller
        name="helpers"
        control={control}
        defaultValue={false}
        render={({ field }) => (
          <FormControlLabel
            label="Auto-create helpers?"
            labelPlacement="top"
            sx={{ marginLeft: 0, marginRight: 0, width: 151 }}
            control={<Checkbox {...field} size="medium" />}
            title="NB! If checked the gatherer will automatically try to create the metric fetching helpers (CPU load monitoring, stat_statements, etc) - requires SUPERUSER to succeed"
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
            labelPlacement="top"
            sx={{ marginLeft: 0, marginRight: 0, width: 151 }}
            control={<Checkbox {...field} size="medium" />}
            title="Fetch metrics only if master / primary"
          />
        )}
      />
      <Controller
        name="md_is_enabled"
        control={control}
        defaultValue={true}
        render={({ field }) => (
          <FormControlLabel
            label="Enabled?"
            labelPlacement="top"
            sx={{ marginLeft: 0, marginRight: 0, width: 151 }}
            control={<Checkbox {...field} size="medium" checked={field.value} />}
            title="A tip - uncheck when leaving 'dbname' empty to review all found DBs before activation"
          />
        )}
      />
        </Box>*/}
    </>
  )
}
