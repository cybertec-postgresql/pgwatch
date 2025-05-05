---
title: Endpoints
---

# pgwatch3 API – Endpoint overview

## Public endpoints (without authentication)

| Path             | Method  | Description                                            |
|------------------|---------|--------------------------------------------------------|
| `/login`         | POST    | Login endpoint, returns Token                          |
| `/liveness`      | GET     | Health check: is the service generally accessible?     |
| `/readiness`     | GET     | Ready check: is the service ready to process requests? |

---

## Authenticated endpoints (Token required)

### Sources

| Path             | Method      | Description                          |
|------------------|-------------|--------------------------------------|
| `/source`        | GET         | List of all configured sources       |
| `/source`        | POST        | Add new source                       |
| `/source`        | PUT         | Update existing source               |
| `/source`        | DELETE      | Delete source                        |

### Check the connection

| Path             | Method      | Description                          |
|------------------|-------------|--------------------------------------|
| `/test-connect`  | POST        | Tests the connection from pgWatch to a DB based on the transmitted connection data |

### Metriken

| Path             | Method      | Description                          |
|------------------|-------------|--------------------------------------|
| `/metric`        | GET         | List of all available metrics        |

### Presets

| Path             | Method      | Description                          |
|------------------|-------------|--------------------------------------|
| `/preset`        | GET         |  List of predefined presets          |

### Logs (WebSocket)

| Path             | Method      | Description                          |
|------------------|-------------|--------------------------------------|
| `/log`           | GET (WS)    | Live log stream via WebSocket        |

---

## Note

All authenticated endpoints require a valid login. The login provides a session cookie, which must be sent in the ‘Cookie’ header for further calls.

---

### `GET /source`

**Description:**  
Lists all configured data sources.

- **Method:** `GET`
- **Authentication:**  Yes (Token-Handling)
- **Header:**
  ```http
  -H "token: <your-token>"
  ``` 

**Example-Answer:**
```json
[
  {
    "Name": "my-db-source",
    "Group": "default",
    "ConnStr": "postgresql://user:password@hostname:5432/dbname",
    "Metrics": {},
    "MetricsStandby": {},
    "Kind": "postgres-continuous-discovery",
    "IncludePattern": "",
    "ExcludePattern": "",
    "PresetMetrics": "standard",
    "PresetMetricsStandby": "",
    "IsSuperuser": false,
    "IsEnabled": true,
    "CustomTags": {},
    "HostConfig": {
      "DcsType": "",
      "DcsEndpoints": null,
      "Scope": "",
      "Namespace": "",
      "Username": "",
      "Password": "",
      "CAFile": "",
      "CertFile": "",
      "KeyFile": "",
      "LogsGlobPath": "",
      "LogsMatchRegex": "",
      "PerMetricDisabledTimes": null
    },
    "OnlyIfMaster": false
  }
]
---

### `POST /source`

**Description:**  
Add a new data source.

- **Method:** `POST`
- **Authentication:**  Yes (Token-Handling)
- **Header:**
  ```http
  -H "token: <your-token>"
  -H "Content-Type: application/json"
  ``` 
- **data:**
    ```json
    {
        "Name": "unique_name_1",
        "Group": "cluster-s3",
        "ConnStr": "postgresql://postgres:oYPYMAxc9jP7Xyh3tzW7Qzcy2h9zJIxQS01O6JNbTEqxtnJf719UOKk85vRk4s23@cluster-s3-1.cpo.svc.cluster.local:5432/postgres",
        "Kind": "postgres-continuous-discovery",
        "IsEnabled": true,
        "IsSuperuser": false,
        "Metrics": {},
        "MetricsStandby": {},
        "PresetMetrics": "standard",
        "PresetMetricsStandby": "",
        "OnlyIfMaster": false,
        "CustomTags": {},
        "IncludePattern": "",
        "ExcludePattern": "",
        "HostConfig": {}
    }
    ```
**Example-Answer:**
```json
```

---

### `PUT /source`

**Description:**  
Add a new data source.

- **Method:** `PUT`
- **Authentication:**  Yes (Token-Handling)
- **Header:**
  ```http
  -H "token: <your-token>"
  -H "Content-Type: application/json"
  ``` 
- **data:**
    ```json
    {
        "Name": "unique_name_1",
        "Group": "cluster-s3",
        "ConnStr": "postgresql://postgres:oYPYMAxc9jP7Xyh3tzW7Qzcy2h9zJIxQS01O6JNbTEqxtnJf719UOKk85vRk4s23@cluster-s3-1.cpo.svc.cluster.local:5432/postgres",
        "Kind": "postgres-continuous-discovery",
        "IsEnabled": true,
        "IsSuperuser": false,
        "Metrics": {},
        "MetricsStandby": {},
        "PresetMetrics": "standard",
        "PresetMetricsStandby": "",
        "OnlyIfMaster": false,
        "CustomTags": {},
        "IncludePattern": "",
        "ExcludePattern": "",
        "HostConfig": {}
    }
    ```
**Example-Answer:**
```json
```

---

### `DELETE /source`

**Description:**  
Löscht eine bestehende Datenquelle anhand ihres Namens.

- **Methode:** `DELETE`  
- **Authentication:**  Yes (Token-Handling)
- **Header:**
  ```http
  -H "token: <your-token>"
  -H "Content-Type: application/json"
  ``` 
- **Query-Parameter:**
  - `name` – Unique name of the cluster to be deleted

