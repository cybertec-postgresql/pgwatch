const capitalized = (str: string) => `${str.charAt(0).toUpperCase()}${str.slice(1)}`;

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

const sslMode = ["disable", "require", "verify-ca", "verify-full"];

const sslModeOptions = sslMode.map(mode => ({value: mode, label: capitalized(mode)}));

const presetConfigs = ["aurora", "azure", "basic", "exhaustive", "full", "full_influx", 
"gce", "minimal", "pgbouncer",  "pgpool",
"prometheus",
"prometheus-async", "rds", "standard", "superuser_no_python", "unprivileged"
];

const presetConfigsOptions = presetConfigs.map(preset => ({ value: preset, label: preset }));

export { dbTypeOptions, passwordEncryptionOptions, sslModeOptions, presetConfigsOptions };
