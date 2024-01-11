package sinks

// Package sinks provides functionality to store monitored data in different ways.
//
// At the moment we provide sink connectors for PostgreSQL and flavours, Prometheus and plain JSON files.
//
// To ensure the simultaneous storage of data in several storages, the `MultiWriter` class is implemented.
