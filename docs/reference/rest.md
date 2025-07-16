# REST API Reference

pgwatch provides a RESTful API for managing monitoring sources, metrics, and presets. All endpoints require authentication unless explicitly noted.

## Authentication

Most endpoints require authentication via JWT token. Obtain a token using the login endpoint:

```bash
$ curl -X POST http://localhost:8080/login \
  -H "Content-Type: application/json" \
  -d '{"user": "your_username", "password": "your_password"}'

eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdXRob3JpemVkIjp0cnVlLCJleHAiOjE3NTI3MDY0OTYsInVzZXJuYW1lIjoieW91cl91c2VybmFtZSJ9.sPpNgpqtjZJqNfgfmypdR3rvlPQxxMtsg2v2WLPVbUA
```

Use the returned token in the Authorization header for subsequent requests:

```bash
# Set token as environment variable (copy the token from login response)
export TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdXRob3JpemVkIjp0cnVlLCJleHAiOjE3NTI3MDY0OTYsInVzZXJuYW1lIjoieW91cl91c2VybmFtZSJ9.sPpNgpqtjZJqNfgfmypdR3rvlPQxxMtsg2v2WLPVbUA"

# Or capture token automatically in one command
TOKEN=$(curl -s -X POST http://localhost:8080/login \
  -H "Content-Type: application/json" \
  -d '{"user": "your_username", "password": "your_password"}')

# Use in requests (note: pgwatch uses 'Token' header, not 'Authorization: Bearer')
curl -H "Token: $TOKEN" http://localhost:8080/source
```

## Health Check APIs

### Liveness probe

Check if the service is running (no authentication required).

```bash
curl -X GET http://localhost:8080/liveness
```

**Response:** `{"status": "ok"}` if service is alive

### Readiness probe

Check if the service is ready to serve requests (no authentication required).

```bash
$ curl -X GET http://localhost:8080/readiness

{"status": "ok"}
```

**Response:** `{"status": "ok"}` if service is ready

## Sources API

### List all sources

Get all monitoring sources.

```bash
$ curl -H "Token: $TOKEN" -X GET http://localhost:8080/source
  
```

**Response:** JSON array of source objects

### Create or update source

Add a new monitoring source or update an existing one.

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

### Get specific source

Retrieve a specific source by name.

```bash
$ curl -H "Token: $TOKEN" -X GET http://localhost:8080/source/my-postgres
  
```

**Response:** JSON object with source details

### Update specific source

Update an existing source using PUT method.

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

### Delete specific source

Remove a monitoring source.

```bash
$ curl -H "Token: $TOKEN" -X DELETE http://localhost:8080/source/my-postgres
  
```

### Test connection

Test connectivity to a PostgreSQL instance. This endpoint allows you to verify that the connection string is valid and the database is reachable from the pgwatch environment.

```bash
$ curl -X POST http://localhost:8080/test-connect \
  -H "Token: $TOKEN" \
  -H "Content-Type: application/json" \
  -d 'postgresql://user:pass@localhost:5432/dbname'
```

## Metrics API

### List all metrics

Get all available metrics definitions.

```bash
$ curl -H "Token: $TOKEN" -X GET http://localhost:8080/metric
  
```

**Response:** JSON array of metric objects

### Create or update metric

Add a new metric definition or update an existing one.

```bash
$ curl -X POST http://localhost:8080/metric \
  -H "Token: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "custom_metric",
    "SQLs": {
        "11": "SELECT count(*) as active_connections FROM pg_stat_activity"
        },
    "Description": "Number of active connections"
  }'
```

### Get specific metric

Retrieve a specific metric definition by name.

```bash
$ curl -H "Token: $TOKEN" -X GET http://localhost:8080/metric/custom_metric
  
```

**Response:** JSON object with metric definition

### Update specific metric

Update an existing metric definition using PUT method.

```bash
$ curl -X PUT http://localhost:8080/metric/custom_metric \
  -H "Token: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "Name": "custom_metric",
    "SQLs": { 
        "11": "SELECT count(*) as connections FROM pg_stat_activity WHERE state = '\''active'\''"
    },
    "Description": "Number of active connections (updated)"
  }'
```

### Delete specific metric

Remove a metric definition.

```bash
curl -H "Token: $TOKEN" -X DELETE http://localhost:8080/metric/custom_metric
```

## Presets API

### List all presets

Get all available metric presets.

```bash
$ curl -X GET http://localhost:8080/preset \
  -H "Token: $TOKEN"
```

**Response:** JSON array of preset objects

### Create or update preset

Add a new preset or update an existing one.

```bash
$ curl -X POST http://localhost:8080/preset \
  -H "Token: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "Name": "custom_preset",
    "Description": "Custom monitoring preset",
    "Metrics": {
      "db_stats": 60,
      "table_stats": 300,
      "custom_metric": 120
    }
  }'
```

### Get specific preset

Retrieve a specific preset by name.

```bash
$ curl -X GET http://localhost:8080/preset/custom_preset \
  -H "Token: $TOKEN"
```

**Response:** JSON object with preset definition

### Update specific preset

Update an existing preset using PUT method.

```bash
$ curl -X PUT http://localhost:8080/preset/custom_preset \
  -H "Token: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "Name": "custom_preset",
    "Description": "Updated custom monitoring preset",
    "Metrics": {
      "db_stats": 30,
      "table_stats": 600,
      "index_stats": 300
    }
  }'
```

### Delete specific preset

Remove a preset definition.

```bash
$ curl -X DELETE http://localhost:8080/preset/custom_preset \
  -H "Token: $TOKEN"
```

## HTTP Status Codes

- `200 OK` - Request successful
- `201 Created` - Resource created successfully
- `400 Bad Request` - Invalid request parameters
- `401 Unauthorized` - Authentication required or invalid
- `404 Not Found` - Resource not found
- `405 Method Not Allowed` - HTTP method not supported for endpoint
- `500 Internal Server Error` - Server error

## Options Requests

All resource endpoints support OPTIONS requests to discover allowed methods:

```bash
$ curl -X OPTIONS http://localhost:8080/source/my-postgres \
  -H "Token: $TOKEN"
```

**Response:** `Allow` header with supported methods (e.g., `GET, PUT, DELETE, OPTIONS`)
