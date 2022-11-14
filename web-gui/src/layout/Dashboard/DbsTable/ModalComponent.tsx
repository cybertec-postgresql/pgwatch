import CloseIcon from "@mui/icons-material/Close";
import DoneIcon from "@mui/icons-material/Done";
import {
  Backdrop, Box, Button, Checkbox, Fade, FormControlLabel,
  Modal, SxProps, TextField, Theme, Typography
} from "@mui/material";
import { Controller, SubmitHandler, useForm } from "react-hook-form";
import {
  DbTypeComponent, MetricsConfigComponent, PasswordEncryptionComponent,
  SslModeComponent, StandbyConfigComponent
} from "./SelectComponents";
import { MultilineTextField, SimpleTextField } from "./TextFieldComponents";

const modalStyle: SxProps<Theme> = {
  position: "absolute",
  top: "50%",
  left: "50%",
  transform: "translate(-50%, -50%)",
  width: 570,
  bgcolor: "background.paper",
  boxShadow: 24,
  p: 4,
  height: 650,
  overflowY: "auto"
};

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
}

export const ModalComponent = ({ open, setOpen, handleAlertOpen, data }: Props) => {
  const { handleSubmit, control, formState: { errors }, setValue } = useForm<IFormInput>();

  const onSubmit: SubmitHandler<IFormInput> = result => {
    if(data) {
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

  return (
    <Box>
      <Modal
        open={open}
        closeAfterTransition
        BackdropComponent={Backdrop}
        BackdropProps={{
          timeout: 750,
        }}
      >
        <Fade in={open}>
          <Box sx={modalStyle}>
            <Typography id="modalTitle" variant="h5" component="h2">
              {data ? "Edit database instance" : "Create new database instance"}
            </Typography>
            <form onSubmit={handleSubmit(onSubmit)}>
              <Box sx={{ display: "grid", rowGap: "15px", marginTop: "15px" }}>
                <Box sx={{ display: "flex", justifyContent: "space-between" }}>
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
                </Box>
                <Box sx={{ display: "flex", justifyContent: "space-between" }}>
                  <Button fullWidth type="submit" size="medium" variant="outlined" startIcon={<DoneIcon />} sx={{ marginRight: "10px" }}>Submit</Button>
                  <Button fullWidth onClick={() => setOpen(false)} size="medium" variant="contained" startIcon={<CloseIcon />}>Cancel</Button>
                </Box>
              </Box>
            </form>
          </Box>
        </Fade>
      </Modal>
    </Box >
  );
};
