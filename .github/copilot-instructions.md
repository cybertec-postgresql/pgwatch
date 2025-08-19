# GitHub Copilot Instructions for `pgwatch`

## Overview
`pgwatch` is a PostgreSQL monitoring solution designed to provide insights into database performance and health. The project is structured as a multi-component system with Docker-based deployment for both development and production environments. Key components include:

- **Configuration and Metrics Database**: Stores monitoring configurations and collected metrics.
- **pgwatch Monitoring Agent**: Collects metrics from PostgreSQL instances.
- **Web UI**: Provides an interface for managing configurations and viewing metrics.
- **Grafana Dashboards**: Visualizes collected metrics.

## Key Developer Workflows

### Generating Protobuf Files
- Protobuf files are located in `api/pb/`.
- To regenerate them, use the `Generate Proto` task or run:
  ```bash
  go generate ./api/pb/
  ```

### Building the Project

- To build the project from source:
    ```bash
    go build ./cmd/pgwatch/
    ```

- If changes made to the webui, first rebuild the webui
    ```
    cd webui && yarn install && yarn build && cd ..
    ```

### Running the Project
- To start the `pgwatch` agent:
  ```bash
  go run ./cmd/pgwatch/
  ```
  This command starts the `pgwatch` agent, which will use default command line options.
  Tweak the command line options as needed for your environment.

### Testing
- To run unit tests:
  ```bash
  go test -failfast -p 1 -timeout=300s -parallel=1 ./... -coverprofile='coverage.out'
  ```

- To run particular test package:
  ```bash
  go test -failfast -p 1 -timeout=300s -parallel=1 ./internal/reaper -coverprofile='coverage.out'
  ```

### Viewing Coverage Reports
- After running tests, generate a coverage report:
  ```bash
  go tool cover -html='coverage.out'
  ```

### Debugging
- To restart the `pgwatch` agent after making changes:
  ```bash
  go build ./cmd/pgwatch/ && go run ./cmd/pgwatch/
  ```

### Logs
- Logs will be printed directly to the console when running the agent using `go run` unless configured to write to a file.

## Project-Specific Conventions

### Concepts
- described in docs/concept/components.md
- main are: 
    - source means where the metrics are collected from, e.g. PostgreSQL.
    - sink means where the metrics are sent, e.g. JSON file, gRPC, PostgreSQL, TimescaleDB, or Prometheus.
    - metric means a single piece of data collected from the source, such as query execution time or connection count.
    - reaper the main component responsible for manaing the lifecycle of metrics and configurations, ensuring they are up-to-date and relevant.

### Code Organization
- **`api/`**: Contains Protobuf definitions and generated files.
- **`cmd/`**: Entry points for different components.
- **`internal/`**: Core libraries and utilities.
- **`docker/`**: Docker Compose files and related scripts.
- **`docs/`**: Documentation, including developer guides and user manuals.
- **`grafana/`**: Grafana dashboard definitions.

### External Dependencies
- **Docker**: Used for containerization and orchestration (optional).
- **Grafana**: For visualizing metrics (optional).
- **Prometheus**: For collecting and storing metrics (optional).
- **TimescaleDB**: An extension of PostgreSQL for time-series data (optional).
- **PostgreSQL**: As the monitored database and for storing metrics.
- **Go**: The primary programming language for the project.
