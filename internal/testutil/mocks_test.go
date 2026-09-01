package testutil_test

import (
	"context"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v6/api/pb"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/sources"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/testutil"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestReceiver_UpdateMeasurements(t *testing.T) {
	receiver := &testutil.Receiver{}
	ctx := context.Background()

	t.Run("valid message", func(t *testing.T) {
		data, err := structpb.NewStruct(map[string]any{"test": "value"})
		require.NoError(t, err)
		msg := &pb.MeasurementEnvelope{
			DBName: "Db",
			Data:   []*structpb.Struct{data},
		}
		reply, err := receiver.UpdateMeasurements(ctx, msg)
		assert.NoError(t, err)
		assert.NotNil(t, reply)
	})

	t.Run("empty data", func(t *testing.T) {
		msg := &pb.MeasurementEnvelope{
			DBName: "Db",
			Data:   []*structpb.Struct{},
		}
		reply, err := receiver.UpdateMeasurements(ctx, msg)
		assert.Error(t, err)
		assert.Nil(t, reply)
		assert.EqualError(t, err, "empty message")
	})

	t.Run("invalid db name", func(t *testing.T) {
		data, err := structpb.NewStruct(map[string]any{"test": "value"})
		require.NoError(t, err)
		msg := &pb.MeasurementEnvelope{
			DBName: "WrongDb",
			Data:   []*structpb.Struct{data},
		}
		reply, err := receiver.UpdateMeasurements(ctx, msg)
		assert.Error(t, err)
		assert.Nil(t, reply)
		assert.EqualError(t, err, "invalid message")
	})
}

func TestReceiver_SyncMetric(t *testing.T) {
	receiver := &testutil.Receiver{}
	ctx := context.Background()

	t.Run("valid sync request", func(t *testing.T) {
		syncReq := &pb.SyncReq{
			Operation: pb.SyncOp_AddOp,
		}
		reply, err := receiver.SyncMetric(ctx, syncReq)
		assert.NoError(t, err)
		assert.NotNil(t, reply)
	})

	t.Run("nil sync request", func(t *testing.T) {
		reply, err := receiver.SyncMetric(ctx, nil)
		assert.Error(t, err)
		assert.Nil(t, reply)
		assert.EqualError(t, err, "nil sync request")
	})

	t.Run("invalid operation", func(t *testing.T) {
		syncReq := &pb.SyncReq{
			Operation: pb.SyncOp_InvalidOp,
		}
		reply, err := receiver.SyncMetric(ctx, syncReq)
		assert.Error(t, err)
		assert.Nil(t, reply)
		assert.EqualError(t, err, "invalid sync request")
	})
}

func TestReceiver_DefineMetrics(t *testing.T) {
	receiver := &testutil.Receiver{}
	ctx := context.Background()

	t.Run("valid metrics struct", func(t *testing.T) {
		metricsStruct, err := structpb.NewStruct(map[string]any{
			"metric1": "value1",
		})
		assert.NoError(t, err)

		reply, err := receiver.DefineMetrics(ctx, metricsStruct)
		assert.NoError(t, err)
		assert.NotNil(t, reply)
		assert.Equal(t, "metrics defined successfully", reply.Logmsg)
	})

	t.Run("nil metrics struct", func(t *testing.T) {
		reply, err := receiver.DefineMetrics(ctx, nil)
		assert.Error(t, err)
		assert.Nil(t, reply)
		assert.EqualError(t, err, "nil metrics struct")
	})

	t.Run("empty metrics struct", func(t *testing.T) {
		metricsStruct := &structpb.Struct{}
		reply, err := receiver.DefineMetrics(ctx, metricsStruct)
		assert.Error(t, err)
		assert.Nil(t, reply)
		assert.EqualError(t, err, "empty metrics struct")
	})
}

func TestMockMetricsReaderWriter(t *testing.T) {
	testData := &metrics.Metrics{
		MetricDefs: map[string]metrics.Metric{"foo": {Description: "bar"}},
		PresetDefs: map[string]metrics.Preset{"foo": {Description: "bar"}},
	}
	called := false

	mock := testutil.MockMetricsReaderWriter{
		GetMetricsFunc: func() (*metrics.Metrics, error) {
			called = true
			return testData, nil
		},
		WriteMetricsFunc: func(*metrics.Metrics) error {
			called = true
			return nil
		},
		UpdateMetricFunc: func(string, metrics.Metric) error {
			called = true
			return nil
		},
		DeleteMetricFunc: func(string) error {
			called = true
			return nil
		},
		CreateMetricFunc: func(string, metrics.Metric) error {
			called = true
			return nil
		},
		CreatePresetFunc: func(string, metrics.Preset) error {
			called = true
			return nil
		},
		UpdatePresetFunc: func(string, metrics.Preset) error {
			called = true
			return nil
		},
		DeletePresetFunc: func(string) error {
			called = true
			return nil
		},
	}

	t.Run("GetMetrics", func(t *testing.T) {
		called = false
		metrics, err := mock.GetMetrics()
		assert.NoError(t, err)
		assert.Equal(t, true, called)
		assert.Equal(t, testData, metrics)
	})

	t.Run("WriteMetrics", func(t *testing.T) {
		called = false
		err := mock.WriteMetrics(testData)
		assert.NoError(t, err)
		assert.Equal(t, true, called)
	})

	t.Run("UpdateMetric", func(t *testing.T) {
		called = false
		err := mock.UpdateMetric("foo", testData.MetricDefs["foo"])
		assert.NoError(t, err)
		assert.Equal(t, true, called)
	})

	t.Run("DeleteMetric", func(t *testing.T) {
		called = false
		err := mock.DeleteMetric("foo")
		assert.NoError(t, err)
		assert.Equal(t, true, called)
	})

	t.Run("CreateMetric", func(t *testing.T) {
		called = false
		err := mock.CreateMetric("foo", testData.MetricDefs["foo"])
		assert.NoError(t, err)
		assert.Equal(t, true, called)
	})

	t.Run("CreatePreset", func(t *testing.T) {
		called = false
		err := mock.CreatePreset("foo", testData.PresetDefs["foo"])
		assert.NoError(t, err)
		assert.Equal(t, true, called)
	})

	t.Run("UpdatePreset", func(t *testing.T) {
		called = false
		err := mock.UpdatePreset("foo", testData.PresetDefs["foo"])
		assert.NoError(t, err)
		assert.Equal(t, true, called)
	})

	t.Run("DeletePreset", func(t *testing.T) {
		called = false
		err := mock.DeletePreset("foo")
		assert.NoError(t, err)
		assert.Equal(t, true, called)
	})
}

func TestMockSourcesReaderWriter(t *testing.T) {
	testData := sources.Sources{{Name: "foo", ConnStr: "postgres://foo@bar"}}
	called := false
	mock := testutil.MockSourcesReaderWriter{
		GetSourcesFunc: func() (sources.Sources, error) {
			called = true
			return testData, nil
		},
		WriteSourcesFunc: func(sources.Sources) error {
			called = true
			return nil
		},
		UpdateSourceFunc: func(sources.Source) error {
			called = true
			return nil
		},
		CreateSourceFunc: func(sources.Source) error {
			called = true
			return nil
		},
		DeleteSourceFunc: func(string) error {
			called = true
			return nil
		},
	}

	t.Run("GetSources", func(t *testing.T) {
		called = false
		sources, err := mock.GetSources()
		assert.NoError(t, err)
		assert.Equal(t, true, called)
		assert.Equal(t, testData, sources)
	})

	t.Run("WriteSources", func(t *testing.T) {
		called = false
		err := mock.WriteSources(testData)
		assert.NoError(t, err)
		assert.Equal(t, true, called)
	})

	t.Run("UpdateSource", func(t *testing.T) {
		called = false
		err := mock.UpdateSource(testData[0])
		assert.NoError(t, err)
		assert.Equal(t, true, called)
	})

	t.Run("DeleteSource", func(t *testing.T) {
		called = false
		err := mock.DeleteSource("foo")
		assert.NoError(t, err)
		assert.Equal(t, true, called)
	})

	t.Run("CreateSource", func(t *testing.T) {
		called = false
		err := mock.CreateSource(testData[0])
		assert.NoError(t, err)
		assert.Equal(t, true, called)
	})
}

// TestBlockingPool covers the wedged-pool fault injector in mocks.go.
// Every BlockingPool method blocks until ctx.Done(); pre-cancelling the
// context makes each call return immediately with ctx.Err() so the
// assertions are deterministic and finish in microseconds.
func TestBlockingPool(t *testing.T) {
	pool := testutil.BlockingPool{}

	cancelledCtx := func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}

	t.Run("Ping", func(t *testing.T) {
		assert.ErrorIs(t, pool.Ping(cancelledCtx()), context.Canceled)
	})

	t.Run("Query", func(t *testing.T) {
		rows, err := pool.Query(cancelledCtx(), "select 1")
		assert.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, rows)
	})

	t.Run("QueryRow", func(t *testing.T) {
		var dst any
		assert.ErrorIs(t, pool.QueryRow(cancelledCtx(), "select 1").Scan(&dst), context.Canceled)
	})

	t.Run("Exec", func(t *testing.T) {
		tag, err := pool.Exec(cancelledCtx(), "x")
		assert.ErrorIs(t, err, context.Canceled)
		assert.Empty(t, tag)
	})

	t.Run("Acquire", func(t *testing.T) {
		conn, err := pool.Acquire(cancelledCtx())
		assert.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, conn)
	})

	t.Run("Close", func(t *testing.T) {
		assert.NotPanics(t, func() { pool.Close() })
	})

	t.Run("SendBatch", func(t *testing.T) {
		br := pool.SendBatch(cancelledCtx(), &pgx.Batch{}).(*testutil.BlockingBatchResults)

		_, err := br.Query()
		assert.ErrorIs(t, err, context.Canceled)

		_, err = br.Exec()
		assert.ErrorIs(t, err, context.Canceled)

		var dst any
		assert.ErrorIs(t, br.QueryRow().Scan(&dst), context.Canceled)

		assert.NoError(t, br.Err()) // before Close
		assert.NoError(t, br.Close())
		assert.ErrorIs(t, br.Err(), context.Canceled) // after Close
	})
}
