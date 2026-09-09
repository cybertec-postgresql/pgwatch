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

## Optional: answering feedback queries

The `Receiver` service defines a fourth, **optional** method, `GetLastMeasurement`. It lets a
resumable pgwatch collector ask what your server already holds, so that after a restart it can
continue from there instead of skipping whatever happened while pgwatch was down:

```proto
rpc GetLastMeasurement(FeedbackReq) returns (FeedbackReply);

message FeedbackReq {
    string DBName = 1;
    string MetricName = 2;
}

message FeedbackReply {
    int64 EpochNs = 1;
}
```

Implementing it is entirely optional. A server that leaves it alone answers `UNIMPLEMENTED`
automatically, pgwatch asks exactly once, and everything else keeps working unchanged.

If you do implement it, return the Unix **nanosecond** timestamp of the newest measurement you
durably hold for that source and metric — durably meaning written, not merely buffered. pgwatch
reads your status code as follows:

| Status | Meaning to pgwatch |
|---|---|
| `OK` with `EpochNs > 0` | The newest measurement you hold |
| `NOT_FOUND` | You support feedback but hold nothing for this pair |
| `UNIMPLEMENTED` | You do not do feedback; pgwatch stops asking for the rest of the process |
| anything else | A transient failure; pgwatch gives up for now and retries later |

Returning `OK` with a zero or negative `EpochNs` is treated as `NOT_FOUND`. Reporting an epoch you
have not actually persisted is the one thing to avoid: a collector may resume after that point and
the measurements in between would be lost.

See [Sinks Options & Parameters](../reference/sinks_options.md#feedback) for how pgwatch combines
answers when several sinks are configured.
