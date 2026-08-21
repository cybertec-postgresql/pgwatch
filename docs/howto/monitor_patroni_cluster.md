---
title: Monitor a Patroni cluster
---

This how-to wires pgwatch into a Patroni-managed PostgreSQL cluster by reading cluster membership from the DCS (Distributed Consensus Store).

For background on what pgwatch's Patroni support does and why it exists, see [Advanced features → Patroni clusters](../concept/advanced_features.md#patroni-clusters).

## Prerequisites

- A running Patroni cluster whose state is stored in etcd, ZooKeeper, or Consul
- Network reachability from the pgwatch host to every DCS node
- Read access to the DCS namespace/scope that Patroni uses

## Steps

1. Add a source of kind `patroni` whose `conn_str` points at the DCS:

    ```yaml
    - name: patroni-prod
      kind: patroni
      conn_str: "etcd://etcd1:2379,etcd2:2379,etcd3:2379/service/batman"
      preset_metrics: patroni
      is_enabled: true
    ```

    The `conn_str` follows the pattern `<scheme>://<host>:<port>[,<host>:<port>...]</namespace>/<scope>`. You may omit the scope to resolve all databases in the namespace, or set it to resolve only one cluster.

2. (etcd only) Pass DCS credentials and TLS material through the connection string if your DCS is secured:

    ```
    etcd://username:password@etcd1:2379/service/batman?ca_file=/etc/ssl/etcd-ca.pem&cert_file=/etc/ssl/client.crt&key_file=/etc/ssl/client.key
    ```

3. Pick the `patroni` preset on the source. It emits the Patroni metric families listed in [Reference: Prometheus source → Presets](../reference/prometheus_source.md#presets) at 30–60 second intervals.

4. If your cluster has standby nodes that you do not want to monitor, enable **Primary mode only** on the source to skip them. This reduces load on the cluster and keeps the dashboards focused on the writer.

5. Wait for the next refresh cycle (default 120 seconds). pgwatch will scan the DCS, discover every member, and start scraping each one.

## Verifying cluster discovery

List the sources pgwatch has resolved:

```bash
curl -H "Token: $TOKEN" http://localhost:8080/source | jq '.[] | select(.Name | startswith("patroni-")) | {Name, ConnStr}'
```

You should see one resolved entry per Patroni member. Each entry will be tracked independently from that point on.

## See also

- [Reference: Source types](../reference/source_types.md) — full `kind:` reference including `patroni`
- [How-to: Monitor a Prometheus exporter](monitor_prometheus_exporter.md) — Patroni exposes its own metrics over HTTP; the `patroni` preset is shared between this source kind and that one
