// Provides functionality to read monitored data from different sources.
//
// Sources defines how to get the information for the monitored databases.
// At the moment, sources definitions support two storages:
// * PostgreSQL database
// * YAML file
//
// * `postgres.go` files cover the functionality for the PostgreSQL database.
// * `yaml.go` files cover the functionality for the YAML file.
// * `resolver.go` implements continuous discovery from patroni and postgres cluster.
// * `types.go` defines the types and interfaces.
// * `sample.sources.yaml` is a sample configuration file.
package sources
