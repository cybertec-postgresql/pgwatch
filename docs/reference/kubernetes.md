---
title: Kubernetes
---

A basic Helm chart templates for installing pgwatch to a Kubernetes
cluster are available as a standalone [repository](https://github.com/cybertec-postgresql/pgwatch-charts). 

!!! notice
    Charts are not considered as a part of pgwatch and
    are not maintained by pgwatch developers.

The corresponding setup can be found in [repository](https://github.com/cybertec-postgresql/pgwatch-charts), 
whereas installation is done via the following commands:

    cd openshift_k8s
    helm install -f chart-values.yml pgwatch ./helm-chart

Please have a look at `helm-chart/values.yaml` to get additional information of configurable options. 