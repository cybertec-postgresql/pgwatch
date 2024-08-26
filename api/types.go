package api

import (
	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
)

type (
	// Metric represents a metric definition
	Metric = metrics.Metric
	// MeasurementEnvelope represents a collection of measurement messages wrapped up
	// with metadata such as metric name, source type, etc.
	MeasurementEnvelope = metrics.MeasurementEnvelope
)
