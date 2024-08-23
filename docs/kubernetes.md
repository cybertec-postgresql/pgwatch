---
title: Kubernetes
---

A basic Helm chart is available for installing pgwatch to a Kubernetes
cluster. The corresponding setup can be found in
[repository](https://github.com/cybertec-postgresql/pgwatch2), whereas installation is done
via the following commands:

    cd openshift_k8s
    helm install -f chart-values.yml pgwatch ./helm-chart

Please have a look at `helm-chart/values.yaml`
to get additional information of configurable options.
