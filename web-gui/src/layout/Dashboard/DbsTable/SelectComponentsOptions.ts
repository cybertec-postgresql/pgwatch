const dbTypeOptions = [
  {
    label: "postgres"
  },
  {
    label: "postgres-continuous-discovery"
  },
  {
    label: "pgbouncer"
  },
  {
    label: "pgpool"
  },
  {
    label: "patroni"
  },
  {
    label: "patroni-continuous-discovery"
  },
  {
    label: "patroni-namespace-discovery"
  },
];

const passwordEncryptionOptions = [
  {
    label: "plain-text"
  },
  {
    label: "aes-gcm-256"
  }
];

const sslModeOptions = [
  {
    label: "disable"
  },
  {
    label: "require"
  },
  {
    label: "verify-ca"
  },
  {
    label: "verify-full"
  }
];

const presetConfigsOptions = [
  {
    label: "aurora"
  },
  {
    label: "azure"
  },
  {
    label: "basic"
  },
  {
    label: "exhaustive"
  },
  {
    label: "full"
  },
  {
    label: "full_influx"
  },
  {
    label: "gce"
  },
  {
    label: "minimal"
  },
  {
    label: "pgbouncer"
  },
  {
    label: "pgpool"
  },
  {
    label: "prometheus"
  },
  {
    label: "prometheus-async"
  },
  {
    label: "rds"
  },
  {
    label: "standard"
  },
  {
    label: "superuser_no_python"
  },
  {
    label: "unprivileged"
  }
];

export { dbTypeOptions, passwordEncryptionOptions, sslModeOptions, presetConfigsOptions };
