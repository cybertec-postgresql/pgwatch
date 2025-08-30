---
title: Implement your own gRPC Server
---

To use pgwatch’s [gRPC sink](../concept/components.md#grpc), you must 
provide the URI of a custom gRPC server that integrates with pgwatch using its 
[protobuf definition](https://github.com/cybertec-postgresql/pgwatch/blob/master/api/pb/pgwatch.proto).  
See also [gRPC Sink URI Parameters](../reference/sinks_options.md#grpc).

## pgwatch_rpc_server

[pgwatch_rpc_server](https://github.com/destrex271/pgwatch3_rpc_server/) 
is a **community-maintained** collection of gRPC server implementations for pgwatch.

It provides servers for common data solutions but makes no guarantees about 
their suitability for real production use. Its main purpose is to provide 
examples and building blocks that users can extend to integrate with pgwatch 
and develop their own production-ready gRPC servers.

You can refer to this short [tutorial](https://github.com/destrex271/pgwatch3_rpc_server/blob/main/TUTORIAL.md) 
for guidance on using pgwatch_rpc_server to implement a custom gRPC server sink.