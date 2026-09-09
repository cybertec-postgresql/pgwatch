// Package sinks provides functionality to store monitored data in different ways.
//
// At the moment we provide sink connectors for
//   - PostgreSQL and flavours,
//   - Prometheus,
//   - plain JSON files,
//   - and RPC servers.
//
// To ensure the simultaneous storage of data in several storages, the `MultiWriter` class is implemented.
package sinks
