---
title: Kubernetes
---

A basic Helm chart is available for installing pgwatch3 to a Kubernetes
cluster. The corresponding setup can be found in
[./openshift_k8s/helm-chart]{.title-ref}, whereas installation is done
via the following commands:

    cd openshift_k8s
    helm install -f chart-values.yml pgwatch3 ./helm-chart

Please have a look at
[openshift_k8s/helm-chart/values.yaml](https://github.com/cybertec-postgresql/pgwatch3/blob/master/openshift_k8s/helm-chart/values.yaml)
to get additional information of configurable options.
