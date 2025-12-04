package testutil

import (
	"context"
	"errors"

	"github.com/cybertec-postgresql/pgwatch/v3/api/pb"
	"google.golang.org/protobuf/types/known/structpb"
)

type Receiver struct {
	pb.UnimplementedReceiverServer
}

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
