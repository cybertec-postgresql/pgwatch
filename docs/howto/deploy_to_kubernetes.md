---
title: Deploying pgwatch to Kubernetes
---

> **Note:** the Helm chart lives in a separate [pgwatch-charts](https://github.com/cybertec-postgresql/pgwatch-charts) repository. It is community-maintained and not part of pgwatch itself. The pgwatch project does not ship or version the chart.

This guide walks you through installing pgwatch onto a Kubernetes or OpenShift cluster using the published Helm chart.

## Prerequisites

- A working Kubernetes (or OpenShift) cluster and `kubectl`/`oc` access
- `helm` v3 installed locally
- A namespace prepared for the install (the chart does not create one)

## Steps

1. Clone the chart repository:

    ```bash
    git clone https://github.com/cybertec-postgresql/pgwatch-charts.git
    cd pgwatch-charts/openshift_k8s
    ```

2. Inspect the available configuration values. Every tunable the chart exposes lives in `helm-chart/values.yaml`:

    ```bash
    less helm-chart/values.yaml
    ```

    Copy the file and edit a copy if you want to override any defaults:

    ```bash
    cp chart-values.yml my-values.yml
    $EDITOR my-values.yml
    ```

3. Install (or upgrade) the release. Substitute `pgwatch` with the release name you want and `my-values.yml` with your values file:

    ```bash
    helm install -f my-values.yml pgwatch ./helm-chart
    # or, to upgrade an existing release:
    helm upgrade -f my-values.yml pgwatch ./helm-chart
    ```

4. Verify the install:

    ```bash
    helm status pgwatch
    kubectl get pods -l app.kubernetes.io/name=pgwatch
    ```

## See also

- [Components](../concept/components.md) — what pgwatch components exist and how they fit together
- [Configuration store options](../concept/installation_options.md) — choose between the in-chart PostgreSQL config store or bringing your own
