package testutil_test

import (
	"context"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v3/api/pb"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/testutil"
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
