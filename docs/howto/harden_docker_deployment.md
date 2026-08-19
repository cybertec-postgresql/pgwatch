---
title: Harden a Docker deployment
---

This how-to turns on a set of security toggles that are built into the official pgwatch Docker images but disabled by default. It assumes you want a one-shot `docker run` that exposes Grafana and the Web UI on their default ports with HTTPS and credential protection enabled.

For the rationale behind each toggle, see [Security aspects](../concept/security.md). For every variable used here, see the canonical [CLI & environment variables](../reference/cli_env.md) and [Docker variables](../reference/env_variables.md) references.

## Prerequisites

- A working Docker host
- The pgwatch image pulled locally (`docker pull cybertec/pgwatch`)
- A passphrase to use for AES-GCM encryption of stored connect strings

## Steps

1. Pick a passphrase for connect-string encryption. This is used by `PW_AES_GCM_KEYPHRASE`. Once chosen, store it somewhere safe — you will need it again to decrypt connect strings on a future reinstall.

2. Launch the container with the following security flags enabled:

    - HTTPS for both Grafana and the Web UI (`PW_GRAFANASSL=1`, `PW_WEBSSL=1`)
    - No anonymous Grafana viewing (`PW_GRAFANANOANONYMOUS=1`) and a custom Grafana admin account (`PW_GRAFANAUSER`, `PW_GRAFANAPASSWORD`)
    - No anonymous Web UI access (`PW_WEBNOANONYMOUS=1`) and a custom Web UI admin account (`PW_WEBUSER`, `PW_WEBPASSWORD`)
    - Component logs hidden from the Web UI (`PW_WEBNOCOMPONENTLOGS=1`)
    - AES-GCM encryption for connect strings in the Config DB (`PW_AES_GCM_KEYPHRASE`)

    ```bash
    docker run --name pgwatch -d --restart=unless-stopped \
      -p 3000:3000 -p 8080:8080 \
      -e PW_GRAFANASSL=1 -e PW_WEBSSL=1 \
      -e PW_GRAFANANOANONYMOUS=1 -e PW_GRAFANAUSER=myuser \
      -e PW_GRAFANAPASSWORD=mypass \
      -e PW_WEBNOANONYMOUS=1 -e PW_WEBNOCOMPONENTLOGS=1 \
      -e PW_WEBUSER=myuser -e PW_WEBPASSWORD=mypass \
      -e PW_AES_GCM_KEYPHRASE=qwerty \
      cybertec/pgwatch
    ```

3. Verify that HTTPS works:

    ```bash
    curl -k https://localhost:3000/api/health
    curl -k https://localhost:8080/readiness
    ```

    Both should return `{"status":"ok"}`. The `-k` flag accepts the self-signed certificate generated on first launch.

4. Verify that anonymous access is rejected. Browsing to `https://localhost:3000` should prompt for the `myuser` / `mypass` Grafana credentials; browsing to `https://localhost:8080` should prompt for the `myuser` / `mypass` Web UI credentials.

## Notes

- Self-signed certificates are generated on first launch. For production, replace them with certificates issued by your internal CA — see [Reverse proxy setup](reverse_proxy.md) for an end-to-end TLS path.
- For custom (non-Docker) installs the same env variables apply; pass them to the `pgwatch` binary directly or via systemd `Environment=` lines.
