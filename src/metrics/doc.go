// # Metrics
//
// Code in this folder is responsible for reading and writing metric definitions.
// At the moment, metric definitions support two storages:
//   - PostgreSQL database
//   - YAML file
//
// # Content
//
//   - `postgres*.go` files cover the functionality for the PostgreSQL database.
//   - `yaml*.go` files cover the functionality for the YAML file.
//   - `metrics.yaml` holds all default metrics and presets.
//   - `default.go` provides access to default metrics.
package metrics
