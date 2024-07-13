package sources

// # Sources

// Code in this folder is responsible for reading and writing sources.
// Sources defines how to get the infgormation for the monitored databases.
// At the moment, sources definitions support two storages:
// * PostgreSQL database
// * YAML file

// ## Content

// * `postgres.go` files cover the functionality for the PostgreSQL database.
// * `yaml.go` files cover the functionality for the YAML file.
// * `resolver.go` implements continous discovery from patroni and postgres cluster.
// * `types.go` defines the types and interfaces.
// * `sample.sources.yaml` is a sample configuration file.
