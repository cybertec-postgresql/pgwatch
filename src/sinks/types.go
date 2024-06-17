package sinks

import (
	"github.com/cybertec-postgresql/pgwatch3/metrics"
)

type MeasurementMessage metrics.MeasurementMessage

type WriteRequest struct {
	PgwatchID int
	Msg       metrics.MeasurementMessage
}
