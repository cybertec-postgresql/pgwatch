---
title: Overview
---

# pgwatch3 API documentation

## Overview

This API is used to **manage and configure** the pgwatch3 instance. It enables data sources (PostgreSQL databases) to be managed, available metrics and presets to be viewed, logs to be called up live and the operational readiness of the system to be checked. The interfaces are predominantly authenticated and require a prior login (session-based via cookie).

---

## Endpoints

### Public endpoints (without authentication)

#### `GET /liveness`
Checks whether the server is alive (running, regardless of the internal status).

- **Response:**  
  - `200 OK`: `{"status": "ok"}`
  - `503 Service Unavailable`: `{"status": "unavailable"}`

---

#### `GET /readiness`
Checks whether the server is ready (depending on the internal status, e.g. database connection, background processes).

- **Response:**  
  - `200 OK`: `{"status": "ok"}`
  - `503 Service Unavailable`: `{"status": "busy"}`

---

## Authentifizierung

### `POST /login`

Authenticates a user by returning the authentication token

- **Headers:**
  - `Content-Type: application/json`

- **Body (JSON):**
```json
{
  "username": "admin",
  "password": "admin"
}
```