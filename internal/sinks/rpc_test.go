package sinks_test

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v3/api/pb"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sinks"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

type Receiver struct {
	pb.UnimplementedReceiverServer
}

var ctx = context.Background()

const ServerAddress = "localhost:6060"

func (receiver *Receiver) UpdateMeasurements(_ context.Context, msg *pb.MeasurementEnvelope) (*pb.Reply, error) {
	if len(msg.GetData()) == 0 {
		return nil, errors.New("empty message")
	}
	if msg.GetDBName() != "Db" {
		return nil, errors.New("invalid message")
	}
	return &pb.Reply{}, nil
}

func (receiver *Receiver) SyncMetric(_ context.Context, syncReq *pb.SyncReq) (*pb.Reply, error) {
	if syncReq == nil {
		return nil, errors.New("nil sync request")
	}
	if syncReq.GetOperation() == pb.SyncOp_InvalidOp {
		return nil, errors.New("invalid sync request")
	}
	return &pb.Reply{}, nil
}

func (receiver *Receiver) DefineMetrics(_ context.Context, metricsStruct *structpb.Struct) (*pb.Reply, error) {
	if metricsStruct == nil {
		return nil, errors.New("nil metrics struct")
	}
	if metricsStruct.GetFields() == nil {
		return nil, errors.New("empty metrics struct")
	}
	return &pb.Reply{Logmsg: "metrics defined successfully"}, nil
}

func init() {
	lis, err := net.Listen("tcp", ServerAddress)
	if err != nil {
		panic(err)
	}

	server := grpc.NewServer()
	recv := new(Receiver)
	pb.RegisterReceiverServer(server, recv)

	go func() {
		if err := server.Serve(lis); err != nil {
			panic(err)
		}
	}()
}

func TestRPCWrite(t *testing.T) {
	a := assert.New(t)

	rw, err := sinks.NewRPCWriter(ctx, ServerAddress)
	a.NoError(err)

	// no error for valid messages
	msgs := metrics.MeasurementEnvelope{
		DBName: "Db",
		Data:   metrics.Measurements{{"test": 1}},
	}
	err = rw.Write(msgs)
	a.NoError(err)

	// error for invalid messages
	msgs.DBName = "invalid"
	err = rw.Write(msgs)
	a.ErrorIs(err, status.Error(codes.Unknown, "invalid message"))

	// error for empty messages
	err = rw.Write(metrics.MeasurementEnvelope{})
	a.ErrorIs(err, status.Error(codes.Unknown, "empty message"))

	// error for cancelled context
	ctx, cancel := context.WithCancel(ctx)
	rw, err = sinks.NewRPCWriter(ctx, ServerAddress)
	a.NoError(err)
	cancel()
	err = rw.Write(msgs)
	a.Error(err)
}

func TestRPCSyncMetric(t *testing.T) {
	a := assert.New(t)

	rw, err := sinks.NewRPCWriter(ctx, ServerAddress)
	a.NoError(err)

	// no error for valid Sync requests
	err = rw.SyncMetric("Test-DB", "DB-Metric", sinks.AddOp)
	a.NoError(err)

	// error for invalid Sync requests
	err = rw.SyncMetric("", "", sinks.InvalidOp)
	a.ErrorIs(err, status.Error(codes.Unknown, "invalid sync request"))

	// error for cancelled context
	ctx, cancel := context.WithCancel(ctx)
	rw, err = sinks.NewRPCWriter(ctx, ServerAddress)
	a.NoError(err)
	cancel()
	err = rw.SyncMetric("Test-DB", "DB-Metric", sinks.AddOp)
	a.Error(err)
}

func TestRPCDefineMetric(t *testing.T) {
	a := assert.New(t)

	rw, err := sinks.NewRPCWriter(ctx, ServerAddress)
	a.NoError(err)

	// Test that RPCWriter implements MetricsDefiner interface
	var writer sinks.Writer = rw
	definer, ok := writer.(sinks.MetricsDefiner)
	a.True(ok, "RPCWriter should implement MetricsDefiner interface")

	// Test with valid metrics
	testMetrics := &metrics.Metrics{
		MetricDefs: metrics.MetricDefs{
			"test_metric": metrics.Metric{
				SQLs: metrics.SQLs{
					11: "SELECT 1 as test_column",
					12: "SELECT 2 as test_column",
				},
				Description: "Test metric",
				Gauges:      []string{"test_column"},
			},
		},
		PresetDefs: metrics.PresetDefs{
			"test_preset": metrics.Preset{
				Description: "Test preset",
				Metrics:     map[string]float64{"test_metric": 30.0},
			},
		},
	}

	err = definer.DefineMetrics(testMetrics)
	a.NoError(err)

	// Test with empty metrics (should still work)
	emptyMetrics := &metrics.Metrics{
		MetricDefs: make(metrics.MetricDefs),
		PresetDefs: make(metrics.PresetDefs),
	}

	err = definer.DefineMetrics(emptyMetrics)
	a.NoError(err)

	// Test with cancelled context
	ctx, cancel := context.WithCancel(ctx)
	rw, err = sinks.NewRPCWriter(ctx, ServerAddress)
	a.NoError(err)
	cancel()

	writer = rw
	definer = writer.(sinks.MetricsDefiner)
	err = definer.DefineMetrics(testMetrics)
	a.Error(err)
}
