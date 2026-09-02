// Package sinks provides functionality to store monitored data in different ways.
//
// At the moment we provide sink connectors for
//   - PostgreSQL and flavours,
//   - Prometheus,
//   - plain JSON files,
//   - and RPC servers.
//
// To ensure the simultaneous storage of data in several storages, the `MultiWriter` class is implemented.
//
// # Feedback
//
// Sinks may optionally implement [Feedbacker] to report the epoch of the newest
// measurement they already hold for a source/metric pair, so that a stateful,
// resumable collector can continue from there instead of restarting from the
// current instant. The capability is negotiated at two levels: implementing the
// interface declares the sink kind capable, and CanFeedback answers for one
// specific pair.
//
//   - PostgresWriter answers from the measurement tables, bounded by retention.
//   - RPCWriter forwards the question to the remote server; servers that do not
//     implement it are probed once and then left alone.
//   - MultiWriter reports the minimum epoch across capable sinks, so a resume
//     never starves the sink that lags furthest behind.
//   - PrometheusWriter and JSONWriter deliberately do not implement it; see
//     spec/design-sink-feedback.md for why.
//
// Nothing in pgwatch queries feedback yet. Read the caller contract in that
// specification before wiring up a consumer.
package sinks
