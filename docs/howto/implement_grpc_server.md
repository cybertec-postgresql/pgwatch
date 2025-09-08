---
title: Implement your own gRPC Server
---

To use pgwatch’s [gRPC sink](../concept/components.md#grpc), you must 
provide the URI of a custom gRPC server that integrates with pgwatch using its 
[protobuf definition](https://github.com/cybertec-postgresql/pgwatch/blob/master/api/pb/pgwatch.proto).  
See also [gRPC Sink URI Parameters](../reference/sinks_options.md#grpc).

## pgwatch contrib RPC

[pgwatch-contrib/rpc](https://github.com/cybertec-postgresql/pgwatch-contrib/tree/main/rpc) 
is a **community-maintained** collection of gRPC server implementations for pgwatch.

It provides servers for common data solutions but makes no guarantees about 
their suitability for production use. Its main purpose is to provide 
examples and building blocks that users can extend to integrate with pgwatch 
and develop their own production-ready gRPC servers.

For guidance on implementing a custom gRPC server sink, refer to this
[tutorial](https://github.com/cybertec-postgresql/pgwatch-contrib/blob/main/rpc/TUTORIAL.md).