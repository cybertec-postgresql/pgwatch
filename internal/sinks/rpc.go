package sinks

import (
	"context"
	"net/rpc"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
)

func NewRPCWriter(ctx context.Context, address string) (*RPCWriter, error) {
	client, err := rpc.DialHTTP("tcp", address)
	if err != nil {
		return nil, err
	}
	l := log.GetLogger(ctx).WithField("sink", "rpc").WithField("address", address)
	ctx = log.WithLogger(ctx, l)
	rw := &RPCWriter{
		ctx:     ctx,
		address: address,
		client:  client,
	}
	go rw.watchCtx()
	return rw, nil
}

// Sends Measurement Message to RPC Sink
func (rw *RPCWriter) Write(msg metrics.MeasurementEnvelope) error {
	if rw.ctx.Err() != nil {
		return rw.ctx.Err()
	}
	var logMsg string
	if err := rw.client.Call("Receiver.UpdateMeasurements", &msg, &logMsg); err != nil {
		return err
	}
	if len(logMsg) > 0 {
		log.GetLogger(rw.ctx).Info(logMsg)
	}
	return nil
}

func (rw *RPCWriter) SyncMetric(dbUnique, metricName string, op SyncOp) error {
	var logMsg string
	if err := rw.client.Call("Receiver.SyncMetric", &SyncReq{
		Operation:  op,
		DbName:     dbUnique,
		MetricName: metricName,
	}, &logMsg); err != nil {
		return err
	}
	if len(logMsg) > 0 {
		log.GetLogger(rw.ctx).Info(logMsg)
	}
	return nil
}

func (rw *RPCWriter) watchCtx() {
	<-rw.ctx.Done()
	rw.client.Close()
}
