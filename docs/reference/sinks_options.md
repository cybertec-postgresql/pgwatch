# Sinks Options & Parameters

## gRPC:

The gRPC sink supports optional **authentication** and **TLS encryption** over the RPC channel.

**Authentication** - Credentials can be provided using the `username:password` format in the URI string, 
(Ex: `--sink=grpc://user:pwd@localhost:5000/`)
If omitted, defaults to empty string for both username and password.
The values are then forwarded to the gRPC server under the `"username"` and `"password"` fields in the metadata.

**TLS encryption** - Enable TLS by specifying a custom Certificate Authority (CA) file via the `sslrootca` URI parameter, e.g.:
`--sink=grpc://localhost:5000/?sslrootca=/home/user/ca.crt`
If omitted, encryption is not used.

## Prometheus:

The Prometheus sink URI format is:  `prometheus://<host>:<port>/<namespace>`

If you omit host (Ex: `--sink=prometheus://:9187`), server listens on all interfaces and supplied port. 
If you omit namespace, default is `pgwatch`.