# REST API Reference

pgwatch provides a RESTful API for managing monitoring sources, metrics, and presets. All endpoints require authentication unless explicitly noted.

## Authentication

Most endpoints require authentication via JWT token. Obtain a token using the login endpoint:

```bash
$ curl -X POST http://localhost:8080/login \
  -H "Content-Type: application/json" \
  -d '{"user": "your_username", "password": "your_password"}'

eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

Use the returned token in subsequent requests:

```bash
# Set token as environment variable (copy the token from login response)
export TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."

# Or capture token automatically in one command
TOKEN=$(curl -s -X POST http://localhost:8080/login \
  -H "Content-Type: application/json" \
  -d '{"user": "your_username", "password": "your_password"}')

# Use in requests (note: pgwatch uses 'Token' header, not 'Authorization: Bearer')
$ curl -H "Token: $TOKEN" http://localhost:8080/source
```

## Health Check APIs

### Liveness probe

Check if the service is running (no authentication required).

```bash
$ curl -X GET http://localhost:8080/liveness
```

**Response:** `200 OK`

```json
{"status": "ok"}
```

### Readiness probe

Check if the service is ready to serve requests (no authentication required).

```bash
$ curl -X GET http://localhost:8080/readiness
```

**Response:** `200 OK`

```json
{"status": "ok"}
```

## API Patterns

pgwatch uses different request patterns for different resource types:

### Collection vs Item Endpoints

- **Collection endpoints** (`/metric`, `/preset`): Used for listing all resources with GET and creating new resources with POST
- **Item endpoints** (`/metric/{name}`, `/preset/{name}`): Used for reading, updating, or deleting specific resources

### Request Body Formats

**Sources** use direct object format:

```json
{
  "Name": "my-postgres",
  "Kind": "postgres",
  "ConnStr": "postgresql://...",
  "IsEnabled": true
}
```

**Metrics and Presets** use collection format for creation (POST to collection endpoint):

```json
{
  "resource_name": {
    "field1": "value1",
    "field2": "value2"
  }
}
```

But use direct object format for updates (PUT to item endpoint):

```json
{
  "field1": "updated_value1",
  "field2": "updated_value2"
}
```

!!! Note
    When using a YAML folder-based configuration for metrics and/or sources, 
    pgwatch operates in **read-only** mode for metrics/presets and/or sources.  
    Attempting to update, create, or delete metrics/presets or sources will result in an error.


## Sources API

### List all sources

Get all monitoring sources.

```bash
$ curl -H "Token: $TOKEN" -X GET http://localhost:8080/source
```

**Response:** `200 OK` — JSON array of source objects

```json
[
  {
    "Name": "my-postgres",
    "Group": "default",
    "ConnStr": "postgresql://user:pass@localhost:5432/dbname",
    "Kind": "postgres",
    "IncludePattern": "",
    "ExcludePattern": "",
    "PresetMetrics": "exhaustive",
    "PresetMetricsStandby": "",
    "IsEnabled": true,
    "CustomTags": {},
    "OnlyIfMaster": false,
    "Metrics": null,
    "MetricsStandby": null
  }
]
```

### Source object fields

| Field | Type | Description |
|-------|------|-------------|
| `Name` | string | Unique identifier for the source |
| `Group` | string | Logical grouping for the source |
| `ConnStr` | string | PostgreSQL connection string |
| `Kind` | string | Source type: `postgres`, `postgres-continuous-discovery`, `pgbouncer`, `pgpool`, or `patroni` |
| `IncludePattern` | string | Regex to include databases (continuous discovery only) |
| `ExcludePattern` | string | Regex to exclude databases (continuous discovery only) |
| `PresetMetrics` | string | Name of a preset to use for primary metrics |
| `PresetMetricsStandby` | string | Name of a preset to use for standby metrics |
| `IsEnabled` | bool | Whether monitoring is active |
| `CustomTags` | object | Key-value pairs added to all measurements from this source |
| `OnlyIfMaster` | bool | Only monitor if the instance is a primary |
| `Metrics` | object | Custom metric-to-interval mapping (alternative to preset) |
| `MetricsStandby` | object | Custom metric-to-interval mapping for standby servers |

### Create source

Add a new monitoring source.

```bash
$ curl -X POST http://localhost:8080/source \
  -H "Token: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "Name": "my-postgres",
    "Kind": "postgres",
    "Group": "default",
    "ConnStr": "postgresql://user:pass@localhost:5432/dbname",
    "PresetMetrics": "exhaustive",
    "IsEnabled": true
  }'
```

**Response:** `201 Created` on success, `409 Conflict` if a source with that name already exists

### Get specific source

Retrieve a specific source by name.

```bash
$ curl -H "Token: $TOKEN" -X GET http://localhost:8080/source/my-postgres
```

**Response:** `200 OK` — JSON object with source details, `404 Not Found` if the source does not exist

```json
{
  "Name": "my-postgres",
  "Group": "default",
  "ConnStr": "postgresql://user:pass@localhost:5432/dbname",
  "Kind": "postgres",
  "PresetMetrics": "exhaustive",
  "IsEnabled": true,
  "CustomTags": {},
  "OnlyIfMaster": false
}
```

### Update specific source

Update an existing source using PUT method. The `Name` in the request body must match the URL parameter.

```bash
$ curl -X PUT http://localhost:8080/source/my-postgres \
  -H "Token: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "Name": "my-postgres",
    "Kind": "postgres",
    "Group": "default",
    "ConnStr": "postgresql://user:pass@localhost:5432/dbname",
    "PresetMetrics": "exhaustive",
    "IsEnabled": false
  }'
```

**Response:** `200 OK` on success, `400 Bad Request` if the name in URL and body do not match

### Delete specific source

Remove a monitoring source.

```bash
$ curl -H "Token: $TOKEN" -X DELETE http://localhost:8080/source/my-postgres
```

**Response:** `200 OK` on success

### Test connection

Test connectivity to a PostgreSQL instance.

```bash
$ curl -X POST http://localhost:8080/test-connect \
  -H "Token: $TOKEN" \
  -H "Content-Type: application/json" \
  -d 'postgresql://user:pass@localhost:5432/dbname'
```

**Response:** `200 OK` if the connection is successful, error message otherwise

## Metrics API

### List all metrics

Get all available metric definitions.

```bash
$ curl -H "Token: $TOKEN" -X GET http://localhost:8080/metric
```

**Response:** `200 OK` — JSON object mapping metric names to definitions

```json
{
  "db_stats": {
    "SQLs": {
      "11": "SELECT ... FROM pg_stat_database ...",
      "16": "SELECT ... FROM pg_stat_database ..."
    },
    "InitSQL": "",
    "NodeStatus": "",
    "Gauges": ["numbackends", "xact_commit"],
    "IsInstanceLevel": false,
    "StorageName": "",
    "Description": "Database statistics from pg_stat_database"
  }
}
```

### Metric object fields

| Field | Type | Description |
|-------|------|-------------|
| `SQLs` | object | Map of PostgreSQL major version (int) to SQL query string |
| `InitSQL` | string | SQL to run once before the first metric collection |
| `NodeStatus` | string | `"primary"` or `"standby"` to restrict which nodes run this metric |
| `Gauges` | array | Column names that represent gauge values (vs. monotonic counters) |
| `IsInstanceLevel` | bool | If true, metric is collected once per instance, not per database |
| `StorageName` | string | Override the metric name used in the storage sink |
| `Description` | string | Human-readable description of the metric |

### Create metric

Add a new metric definition. POST to the collection endpoint expects a map with metric name as key.

!!! Note
    You can create multiple metrics in a single request by providing
    multiple key-value pairs.

```bash
$ curl -X POST http://localhost:8080/metric \
  -H "Token: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "custom_metric": {
      "SQLs": {
        "11": "SELECT count(*) as active_connections FROM pg_stat_activity"
      },
      "Description": "Number of active connections"
    }
  }'
```

**Response:** `201 Created` on success, `409 Conflict` if the metric already exists

**Bulk creation example:**

```bash
$ curl -X POST http://localhost:8080/metric \
  -H "Token: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "metric_one": {
      "SQLs": {"11": "SELECT 1"},
      "Description": "First metric"
    },
    "metric_two": {
      "SQLs": {"11": "SELECT 2"},
      "Description": "Second metric"
    }
  }'
```

### Get specific metric

Retrieve a specific metric definition by name.

```bash
$ curl -H "Token: $TOKEN" -X GET http://localhost:8080/metric/custom_metric
```

**Response:** `200 OK` — JSON object with metric definition, `404 Not Found` if not found

```json
{
  "SQLs": {
    "11": "SELECT count(*) as active_connections FROM pg_stat_activity"
  },
  "Description": "Number of active connections"
}
```

### Update specific metric

Update an existing metric definition using PUT method on the item endpoint.

```bash
$ curl -X PUT http://localhost:8080/metric/custom_metric \
  -H "Token: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "SQLs": { 
        "11": "SELECT count(*) as connections FROM pg_stat_activity WHERE state = $$active$$"
    },
    "Description": "Number of active connections (updated)"
  }'
```

**Response:** `200 OK` on success

### Delete specific metric

Remove a metric definition.

```bash
$ curl -H "Token: $TOKEN" -X DELETE http://localhost:8080/metric/custom_metric
```

**Response:** `200 OK` on success

## Presets API

### List all presets

Get all available metric presets.

```bash
$ curl -H "Token: $TOKEN" -X GET http://localhost:8080/preset
```

**Response:** `200 OK` — JSON object mapping preset names to definitions

```json
{
  "basic": {
    "Description": "Basic set of metrics for lightweight monitoring",
    "Metrics": {
      "db_stats": 60,
      "cpu_load": 60
    }
  },
  "exhaustive": {
    "Description": "All available metrics for detailed monitoring",
    "Metrics": {
      "db_stats": 60,
      "table_stats": 300,
      "index_stats": 300,
      "cpu_load": 60,
      "wal": 60
    }
  }
}
```

### Preset object fields

| Field | Type | Description |
|-------|------|-------------|
| `Description` | string | Human-readable description of the preset |
| `Metrics` | object | Map of metric name to collection interval in seconds |

### Create preset

Add a new preset. POST to the collection endpoint expects a map with preset name as key.

!!! Note
    You can create multiple presets in a single request
    by providing multiple key-value pairs.

```bash
$ curl -X POST http://localhost:8080/preset \
  -H "Token: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "custom_preset": {
      "Description": "Custom monitoring preset",
      "Metrics": {
        "db_stats": 60,
        "table_stats": 300,
        "custom_metric": 120
      }
    }
  }'
```

**Response:** `201 Created` on success, `409 Conflict` if the preset already exists

### Get specific preset

Retrieve a specific preset by name.

```bash
$ curl -H "Token: $TOKEN" -X GET http://localhost:8080/preset/custom_preset
```

**Response:** `200 OK` — JSON object with preset definition, `404 Not Found` if not found

```json
{
  "Description": "Custom monitoring preset",
  "Metrics": {
    "db_stats": 60,
    "table_stats": 300,
    "custom_metric": 120
  }
}
```

### Update specific preset

Update an existing preset using PUT method on the item endpoint.

```bash
$ curl -X PUT http://localhost:8080/preset/custom_preset \
  -H "Token: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "Description": "Updated custom monitoring preset",
    "Metrics": {
      "db_stats": 30,
      "table_stats": 600,
      "index_stats": 300
    }
  }'
```

**Response:** `200 OK` on success

### Delete specific preset

Remove a preset definition.

```bash
$ curl -H "Token: $TOKEN" -X DELETE http://localhost:8080/preset/custom_preset
```

**Response:** `200 OK` on success

## Log Streaming API

### WebSocket log stream

Stream live server logs via WebSocket connection. Useful for real-time monitoring and debugging.

```bash
$ wscat -c ws://localhost:8080/log -H "Token: $TOKEN"
```

This endpoint upgrades the HTTP connection to a WebSocket and streams log entries as they are generated by the pgwatch server.

!!! Note
    This endpoint requires a WebSocket client. Standard HTTP clients like curl
    cannot consume WebSocket streams.

## Options Requests

All resource endpoints support OPTIONS requests to discover allowed methods:

```bash
$ curl -H "Token: $TOKEN" -X OPTIONS http://localhost:8080/source/my-postgres
```

**Response:** `Allow` header with supported methods (e.g., `GET, PUT, DELETE, OPTIONS`)

## HTTP Status Codes

| Code | Meaning |
|------|---------|
| `200 OK` | Request successful |
| `201 Created` | Resource created successfully |
| `400 Bad Request` | Invalid request parameters or mismatched name in URL/body |
| `401 Unauthorized` | Authentication required or token invalid/expired |
| `404 Not Found` | Resource not found |
| `405 Method Not Allowed` | HTTP method not supported for this endpoint |
| `409 Conflict` | Resource already exists (duplicate name on creation) |
| `500 Internal Server Error` | Server error |
