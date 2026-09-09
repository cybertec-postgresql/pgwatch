---
title: Sinks Options & Parameters
---

# Sinks Options & Parameters

- Sinks URIs should be provided to pgwatch via the `--sink` flag, which can be used more than once, see [CLI & Envs](./cli_env.md#sinks).

## PostgreSQL

The PostgreSQL sink URI format is the standard [PostgreSQL connection string](https://www.postgresql.org/docs/current/libpq-connect.html#LIBPQ-CONNSTRING-URIS),
e.g.

```bash
--sink=postgresql://user:pwd@host:port/dbname?sslmode=disable&connect_timeout=10
```

All [standard environment variables](https://www.postgresql.org/docs/current/libpq-envars.html) are supported as well.

## Prometheus

The Prometheus sink URI format is

```bash
--sink=prometheus://host:port/namespace
```

If you omit host, e.g. `--sink=prometheus://:9090`, server listens on all interfaces and supplied port.
If you omit namespace, default is `pgwatch`.

## JSON file

The JSON file sink URI format is

```bash
--sink=jsonfile:///path/to/file.json
```

It should be a valid file path where the JSON data will be written. If the file does not exist, it will be created.

## gRPC

The gRPC sink URI format is

```bash
--sink=grpc://user:pwd@host:port/?sslrootca=/path/to/ca.crt
```

The gRPC sink supports optional **authentication** and **TLS encryption** over the RPC channel.

For authentication, credentials can be provided using the `username:password` format in the URI string,
e.g. `--sink=grpc://user:pwd@localhost:5000/`.
If omitted, defaults to empty string for both username and password.
The values are then forwarded to the gRPC server under the `"username"` and `"password"` fields in the metadata.

Enable TLS by specifying a custom Certificate Authority (CA) file via the `sslrootca` URI parameter, e.g.
`--sink=grpc://localhost:5000/?sslrootca=/home/user/ca.crt`
If omitted, encryption is not used.

## Feedback

Some collectors are *resumable*: what they produce next depends on where the previous run stopped,
not only on the current state of the source. After a restart such a collector needs to know how far
the sink already got, otherwise it silently skips whatever happened while pgwatch was down.

Sinks may therefore answer one question: **what is the epoch of the newest measurement you already
hold for this source and this metric?** Support is negotiated at two levels — whether the sink kind
can answer at all, and whether it can answer for one specific source/metric pair.

| Sink | Feedback | Notes |
|---|---|---|
| PostgreSQL | yes | Reads the newest `time` from the metric table, bounded by `--retention` so partition pruning keeps it cheap |
| gRPC | yes, if the server implements it | Calls `GetLastMeasurement`; servers that do not implement it are probed once and never asked again |
| Prometheus | no | Pull-based: its cache says what pgwatch *offered*, not what a Prometheus server *stored* |
| JSON file | no | Answering would mean scanning rotated, compressed files backwards with no index |

With several `--sink` targets the **oldest** reported epoch wins. Resuming from the newest would
leave the sink that lags furthest behind permanently short of data, whereas resuming from the oldest
only re-sends measurements the leading sink already has. Sinks that cannot answer are skipped and do
not suppress the answer from those that can — they simply receive the replayed span again.

Feedback is on by default and costs nothing until a collector asks. Turn it off with
`--no-sink-feedback` (`$PW_NO_SINK_FEEDBACK`).
