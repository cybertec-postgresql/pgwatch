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
	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGCDSlice(t *testing.T) {
	tests := []struct {
		name string
		vals []int
		want int
	}{
		{"empty", nil, 0},
		{"single", []int{30}, 30},
		{"exhaustive preset intervals", []int{30, 60, 120, 180, 300, 600, 900, 3600, 7200}, 30},
		{"coprime", []int{7, 11, 13}, 1},
		{"all same", []int{60, 60, 60}, 60},
		{"basic preset", []int{60, 120}, 60},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, GCDSlice(tc.vals))
		})
	}
}

func TestCalcTickInterval(t *testing.T) {
	t.Run("exhaustive preset GCD is 30s", func(t *testing.T) {
		sr := &SourceReaper{
			md: &sources.SourceConn{
				Source: sources.Source{
					Metrics: metrics.MetricIntervals{"m1": 30, "m2": 60, "m3": 120, "m4": 300},
				},
			},
		}
		assert.Equal(t, 30*time.Second, sr.calcTickInterval())
	})

	t.Run("GCD floors to minimum 1s", func(t *testing.T) {
		sr := &SourceReaper{
			md: &sources.SourceConn{
				Source: sources.Source{
					Metrics: metrics.MetricIntervals{"m1": 3, "m2": 7},
				},
			},
		}
		assert.Equal(t, time.Second, sr.calcTickInterval())
	})

	t.Run("single metric", func(t *testing.T) {
		sr := &SourceReaper{
			md: &sources.SourceConn{
				Source: sources.Source{
					Metrics: metrics.MetricIntervals{"m1": 60},
				},
			},
		}
		assert.Equal(t, 60*time.Second, sr.calcTickInterval())
	})

	t.Run("empty metrics", func(t *testing.T) {
		sr := &SourceReaper{
			md: &sources.SourceConn{
				Source: sources.Source{
					Metrics: metrics.MetricIntervals{},
				},
			},
		}
		assert.Equal(t, time.Second, sr.calcTickInterval())
	})

	t.Run("standby metrics when in recovery", func(t *testing.T) {
		sr := &SourceReaper{
			md: &sources.SourceConn{
				Source: sources.Source{
					Metrics:        metrics.MetricIntervals{"m1": 30, "m2": 60},
					MetricsStandby: metrics.MetricIntervals{"m1": 120},
				},
				RuntimeInfo: sources.RuntimeInfo{IsInRecovery: true},
			},
		}
		assert.Equal(t, 120*time.Second, sr.calcTickInterval())
	})
}

func TestNewSourceReaper(t *testing.T) {
	r := &Reaper{
		measurementCh:    make(chan metrics.MeasurementEnvelope, 10),
		measurementCache: NewInstanceMetricCache(),
	}
	md := &sources.SourceConn{
		Source: sources.Source{
			Name:    "testdb",
			Kind:    sources.SourcePostgres,
			Metrics: metrics.MetricIntervals{"cpu": 30, "mem": 60, "disk": 120},
		},
	}
	sr := NewSourceReaper(r, md)

	assert.NotNil(t, sr.lastFetch)
	assert.Empty(t, sr.lastFetch)
	assert.Equal(t, r, sr.reaper)
	assert.Equal(t, md, sr.md)
}

func TestSourceReaper_ExecuteBatch(t *testing.T) {
	ctx := log.WithLogger(context.Background(), log.NewNoopLogger())

	metricDefs.MetricDefs["batch_metric_1"] = metrics.Metric{
		SQLs: metrics.SQLs{0: "SELECT 1 as value, 100::bigint as epoch_ns"},
	}
	metricDefs.MetricDefs["batch_metric_2"] = metrics.Metric{
		SQLs: metrics.SQLs{0: "SELECT 2 as value, 200::bigint as epoch_ns"},
	}

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	md := &sources.SourceConn{
		Source: sources.Source{
			Name:    "test_source",
			Kind:    sources.SourcePostgres,
			Metrics: metrics.MetricIntervals{"batch_metric_1": 30, "batch_metric_2": 30},
		},
		Conn: mock,
		RuntimeInfo: sources.RuntimeInfo{
			Version:     120000,
			ChangeState: make(map[string]map[string]string),
		},
	}

	r := &Reaper{
		Options: &cmdopts.Options{
			Metrics: metrics.CmdOpts{},
			Sinks:   sinks.CmdOpts{},
		},
		measurementCh:    make(chan metrics.MeasurementEnvelope, 10),
		measurementCache: NewInstanceMetricCache(),
	}
	sr := NewSourceReaper(r, md)

	rows1 := pgxmock.NewRows([]string{"epoch_ns", "value"}).
		AddRow(time.Now().UnixNano(), int64(100))
	rows2 := pgxmock.NewRows([]string{"epoch_ns", "value"}).
		AddRow(time.Now().UnixNano(), int64(200))
	eb := mock.ExpectBatch()
	eb.ExpectQuery("SELECT 1").WillReturnRows(rows1)
	eb.ExpectQuery("SELECT 2").WillReturnRows(rows2)

	err = sr.executeBatch(ctx, []batchEntry{
		{name: "batch_metric_1", metric: metricDefs.MetricDefs["batch_metric_1"], sql: "SELECT 1 as value, 100::bigint as epoch_ns"},
		{name: "batch_metric_2", metric: metricDefs.MetricDefs["batch_metric_2"], sql: "SELECT 2 as value, 200::bigint as epoch_ns"},
	})
	assert.NoError(t, err)

	received := 0
	for {
		select {
		case msg := <-r.measurementCh:
			assert.Equal(t, "test_source", msg.DBName)
			assert.True(t, msg.MetricName == "batch_metric_1" || msg.MetricName == "batch_metric_2")
			received++
		default:
			goto done
		}
	}
done:
	assert.Equal(t, 2, received, "should have received 2 measurement envelopes")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSourceReaper_RunOneIteration(t *testing.T) {
	ctx, cancel := context.WithCancel(log.WithLogger(context.Background(), log.NewNoopLogger()))

	metricDefs.MetricDefs["run_test_metric"] = metrics.Metric{
		SQLs: metrics.SQLs{0: "SELECT run_test"},
	}

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	md := &sources.SourceConn{
		Source: sources.Source{
			Name:    "run_source",
			Kind:    sources.SourcePostgres,
			Metrics: metrics.MetricIntervals{"run_test_metric": 5},
		},
		Conn: mock,
		RuntimeInfo: sources.RuntimeInfo{
			Version:     120000,
			ChangeState: make(map[string]map[string]string),
		},
	}

	r := &Reaper{
		Options: &cmdopts.Options{
			Metrics: metrics.CmdOpts{},
			Sinks:   sinks.CmdOpts{},
		},
		measurementCh:    make(chan metrics.MeasurementEnvelope, 10),
		measurementCache: NewInstanceMetricCache(),
	}
	sr := NewSourceReaper(r, md)

	// FetchRuntimeInfo sends a query
	mock.ExpectQuery("select /\\* pgwatch_generated \\*/").
		WillReturnError(assert.AnError)

	rows := pgxmock.NewRows([]string{"epoch_ns", "value"}).
		AddRow(time.Now().UnixNano(), int64(42))
	eb := mock.ExpectBatch()
	eb.ExpectQuery("SELECT run_test").WillReturnRows(rows)

	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	sr.Run(ctx)

	select {
	case msg := <-r.measurementCh:
		assert.Equal(t, "run_source", msg.DBName)
		assert.Equal(t, "run_test_metric", msg.MetricName)
	case <-time.After(time.Second):
		t.Error("Expected measurement but timed out")
	}
}

func TestSourceReaper_DetectServerRestart(t *testing.T) {
	sr := &SourceReaper{
		reaper: &Reaper{
			measurementCh: make(chan metrics.MeasurementEnvelope, 10),
		},
		md: &sources.SourceConn{
			Source: sources.Source{Name: "restart_test"},
		},
	}

	// First observation — establish baseline
	data := metrics.Measurements{
		{"epoch_ns": time.Now().UnixNano(), "postmaster_uptime_s": int64(1000)},
	}
	sr.detectServerRestart(t.Context(), data)
	assert.Equal(t, int64(1000), sr.lastUptimeS)
	select {
	case <-sr.reaper.measurementCh:
		t.Error("should not emit restart event on first observation")
	default:
	}

	// Second observation — uptime increased (normal)
	data = metrics.Measurements{
		{"epoch_ns": time.Now().UnixNano(), "postmaster_uptime_s": int64(2000)},
	}
	sr.detectServerRestart(t.Context(), data)
	assert.Equal(t, int64(2000), sr.lastUptimeS)
	select {
	case <-sr.reaper.measurementCh:
		t.Error("should not emit restart event when uptime increases")
	default:
	}

	// Third observation — uptime decreased (restart!)
	data = metrics.Measurements{
		{"epoch_ns": time.Now().UnixNano(), "postmaster_uptime_s": int64(10)},
	}
	sr.detectServerRestart(t.Context(), data)
	assert.Equal(t, int64(10), sr.lastUptimeS)
	select {
	case msg := <-sr.reaper.measurementCh:
		assert.Equal(t, "object_changes", msg.MetricName)
		assert.Contains(t, msg.Data[0]["details"], "restart")
	default:
		t.Error("expected restart event")
	}
}

func TestSourceReaper_FetchSpecialMetric(t *testing.T) {
	ctx := log.WithLogger(context.Background(), log.NewNoopLogger())

	newSR := func(t *testing.T) (*SourceReaper, *sources.SourceConn, pgxmock.PgxPoolIface) {
		t.Helper()
		md, mock := createTestSourceConn(t)
		r := &Reaper{
			Options: &cmdopts.Options{
				Metrics: metrics.CmdOpts{},
				Sinks:   sinks.CmdOpts{},
			},
			measurementCh:    make(chan metrics.MeasurementEnvelope, 10),
			measurementCache: NewInstanceMetricCache(),
		}
		return NewSourceReaper(r, md), md, mock
	}

	sr, _, mock := newSR(t)
	defer mock.Close()

	t.Run("instance_up dispatches measurement on ping success", func(t *testing.T) {
		mock.ExpectPing()
		assert.NoError(t, sr.fetchSpecialMetric(ctx, specialMetricInstanceUp, ""))
		select {
		case msg := <-sr.reaper.measurementCh:
			assert.Equal(t, specialMetricInstanceUp, msg.MetricName)
			assert.Len(t, msg.Data, 1)
			assert.Equal(t, 1, msg.Data[0][specialMetricInstanceUp])
		default:
			t.Error("expected measurement for instance_up")
		}
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("instance_up uses storage name when set", func(t *testing.T) {
		mock.ExpectPing()
		assert.NoError(t, sr.fetchSpecialMetric(ctx, specialMetricInstanceUp, "infra_up"))
		select {
		case msg := <-sr.reaper.measurementCh:
			assert.Equal(t, "infra_up", msg.MetricName)
		default:
			t.Error("expected measurement")
		}
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("change_events dispatches no measurement when no hash defs present", func(t *testing.T) {
		// Doesn't contain additional defs for any of {"sproc_hashes", "table_hashes", "index_hashes", "configuration_hashes", "privilege_hashes"}
		metricDefs.MetricDefs[specialMetricChangeEvents] = metrics.Metric{}
		assert.NoError(t, sr.fetchSpecialMetric(ctx, specialMetricChangeEvents, ""))
		select {
		case <-sr.reaper.measurementCh:
			t.Error("expected no measurement when no changes detected")
		default:
		}
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestSourceReaper_NonPostgresSequential(t *testing.T) {
	ctx := log.WithLogger(context.Background(), log.NewNoopLogger())

	metricDefs.MetricDefs["seq_metric"] = metrics.Metric{
		SQLs: metrics.SQLs{0: "SELECT seq_value"},
	}

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	md := &sources.SourceConn{
		Source: sources.Source{
			Name:    "seq_test_src",
			Kind:    sources.SourcePostgres,
			Metrics: metrics.MetricIntervals{"seq_metric": 30},
		},
		Conn: mock,
		RuntimeInfo: sources.RuntimeInfo{
			Version:     120000,
			ChangeState: make(map[string]map[string]string),
		},
	}

	r := &Reaper{
		Options: &cmdopts.Options{
			Metrics: metrics.CmdOpts{},
			Sinks:   sinks.CmdOpts{},
		},
		measurementCh:    make(chan metrics.MeasurementEnvelope, 10),
		measurementCache: NewInstanceMetricCache(),
	}
	sr := NewSourceReaper(r, md)

	rows := pgxmock.NewRows([]string{"epoch_ns", "value"}).
		AddRow(time.Now().UnixNano(), int64(42))
	mock.ExpectQuery("SELECT seq_value").WithArgs(pgx.QueryExecModeSimpleProtocol).WillReturnRows(rows)

	err = sr.fetchMetric(ctx, batchEntry{name: "seq_metric", metric: metricDefs.MetricDefs["seq_metric"], sql: "SELECT seq_value"})
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
