# Metrics

Code in this folder is responsible for reading and writing metric definitions. 
Ar the moment metric definitions support two storages:
* PostgreSQL database
* YAML file

## Content

* `postgres*.go` files cover the functionality for PostgreSQL database.
* `yaml*.go` files cover the YAML file functionality
* `metrics.yaml` holds all default metrics and presets
* `default.go` provides an access to default metrics
