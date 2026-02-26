package reaper

import (
	"context"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/cmdopts"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sinks"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
	"github.com/stretchr/testify/assert"
)

func TestReaper_WriteMeasurements(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	received := make(chan metrics.MeasurementEnvelope, 1)
	mockSink := &MockSinkWriter{
		WriteFunc: func(msg metrics.MeasurementEnvelope) error {
			received <- msg
			return nil
		},
	}

	r := &Reaper{
		measurementCh: make(chan metrics.MeasurementEnvelope, 1),
		Options: &cmdopts.Options{
			SinksWriter: mockSink,
		},
	}

	go r.WriteMeasurements(ctx)

	msg := metrics.MeasurementEnvelope{DBName: "test"}
	r.measurementCh <- msg

	select {
	case rx := <-received:
		assert.Equal(t, "test", rx.DBName)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for measurement")
	}
}

func TestReaper_Ready(t *testing.T) {
	r := &Reaper{}
	assert.False(t, r.Ready())
	r.ready.Store(true)
	assert.True(t, r.Ready())
}

func TestReaper_AddSysinfoToMeasurements(t *testing.T) {
	r := &Reaper{
		Options: &cmdopts.Options{
			Sinks: sinks.CmdOpts{
				RealDbnameField:       "real_db",
				SystemIdentifierField: "sys_id",
			},
		},
	}
	md := &sources.SourceConn{
		Source: sources.Source{},
		RuntimeInfo: sources.RuntimeInfo{
			RealDbname:       "actual_db",
			SystemIdentifier: "12345",
		},
	}
	data := metrics.Measurements{
		{"col1": "val1"},
	}

	r.AddSysinfoToMeasurements(data, md)
	assert.Equal(t, "actual_db", data[0]["real_db"])
	assert.Equal(t, "12345", data[0]["sys_id"])
}

func TestReaper_WriteMonitoredSources(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := &Reaper{
		measurementCh: make(chan metrics.MeasurementEnvelope, 10),
		monitoredSources: sources.SourceConns{
			&sources.SourceConn{Source: sources.Source{Name: "db1", Group: "g1"}},
		},
	}

	// Change interval to something small for testing
	oldInterval := monitoredDbsDatastoreSyncIntervalSeconds
	monitoredDbsDatastoreSyncIntervalSeconds = 1
	defer func() { monitoredDbsDatastoreSyncIntervalSeconds = oldInterval }()

	go r.WriteMonitoredSources(ctx)

	select {
	case msg := <-r.measurementCh:
		assert.Equal(t, "db1", msg.DBName)
		assert.Equal(t, monitoredDbsDatastoreSyncMetricName, msg.MetricName)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for monitored sources sync")
	}
}

func TestReaper_PrintMemStats(t *testing.T) {
	ctx := log.WithLogger(context.Background(), log.NewNoopLogger())
	r := NewReaper(ctx, &cmdopts.Options{})
	// Just ensure it doesn't panic
	assert.NotPanics(t, func() { r.PrintMemStats() })
}
