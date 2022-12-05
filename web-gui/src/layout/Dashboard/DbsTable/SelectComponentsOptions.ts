const dbType = ["postgres", "postgres-continuous-discovery", "pgbouncer", "pgpool", "patroni", "patroni-continuous-discovery", "patroni-namespace-discovery"];

const dbTypeOptions = dbType.map(type => ({ label: type }));

const passwordEncryption = ["plain-text", "aes-gcm-256"];

const passwordEncryptionOptions = passwordEncryption.map(option => ({ label: option }));

const sslMode = ["disable", "require", "verify-ca", "verify-full"];

const sslModeOptions = sslMode.map(mode => ({ label: mode }));

const presetConfigs = ["aurora", "azure", "basic", "exhaustive", "full", "full_influx",
  "gce", "minimal", "pgbouncer", "pgpool",
  "prometheus",
  "prometheus-async", "rds", "standard", "superuser_no_python", "unprivileged"
];

const presetConfigsOptions = presetConfigs.map(preset => ({ label: preset }));

export { dbTypeOptions, passwordEncryptionOptions, sslModeOptions, presetConfigsOptions };
