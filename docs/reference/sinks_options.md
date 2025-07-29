# Sinks Options & Parameters

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

If you omit host, e.g. `--sink=prometheus://:9187`, server listens on all interfaces and supplied port.
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

For authentication credentials can be provided using the `username:password` format in the URI string,
e.g. `--sink=grpc://user:pwd@localhost:5000/`.
If omitted, defaults to empty string for both username and password.
The values are then forwarded to the gRPC server under the `"username"` and `"password"` fields in the metadata.

Enable TLS by specifying a custom Certificate Authority (CA) file via the `sslrootca` URI parameter, e.g.
`--sink=grpc://localhost:5000/?sslrootca=/home/user/ca.crt`
If omitted, encryption is not used.
