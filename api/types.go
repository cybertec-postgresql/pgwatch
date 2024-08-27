package api

import (
	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sinks"
)

type (
	// Metric represents a metric definition
	Metric = metrics.Metric
	// MeasurementEnvelope represents a collection of measurement messages wrapped up
	// with metadata such as metric name, source type, etc.
	MeasurementEnvelope = metrics.MeasurementEnvelope
	// RPCSyncRequest represents a request to sync metrics with the remote RPC sink
	RPCSyncRequest = sinks.SyncReq
)
