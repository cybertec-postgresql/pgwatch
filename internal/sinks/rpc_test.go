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
)

type Receiver struct {
	pb.UnimplementedReceiverServer
}
var ctx = context.Background()

const ServerAddress = "localhost:6060"

func (receiver *Receiver) UpdateMeasurements(ctx context.Context, msg *pb.MeasurementEnvelope) (*pb.Reply, error) {
	if len(msg.GetData()) == 0 {
		return nil, errors.New("empty message")
	}
	if msg.GetDBName() != "Db" {
		return nil, errors.New("invalid message")
	}
	return &pb.Reply{}, nil
}

func (receiver *Receiver) SyncMetric(ctx context.Context, syncReq *pb.SyncReq) (*pb.Reply, error) {
	if syncReq == nil {
		return nil, errors.New("nil sync request")
	}
	if syncReq.GetOperation() == pb.SyncOp_InvalidOp {
		return nil, errors.New("invalid sync request")
	}
	return &pb.Reply{}, nil
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

// Test begin from here ---------------------------------------------------------

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
	err = rw.SyncMetric("Test-DB", "DB-Metric", pb.SyncOp_AddOp)
	a.NoError(err)

	// error for invalid Sync requests
	err = rw.SyncMetric("", "", pb.SyncOp_InvalidOp)
	a.ErrorIs(err, status.Error(codes.Unknown, "invalid sync request"))

	// error for cancelled context
	ctx, cancel := context.WithCancel(ctx)
	rw, err = sinks.NewRPCWriter(ctx, ServerAddress)
	a.NoError(err)
	cancel()
	err = rw.SyncMetric("Test-DB", "DB-Metric", pb.SyncOp_AddOp)
	a.Error(err)
}