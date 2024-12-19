package sinks

import (
	"context"
	"net/rpc"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
)

// RPCWriter is a sink that sends metric measurements to a remote server using the RPC protocol.
// Remote server should implement the Receiver interface. It's up to the implementer to define the
// behavior of the server. It can be a simple logger, external storage, alerting system,
// or an analytics system.
type RPCWriter struct {
	ctx     context.Context
	address string
	client  *rpc.Client
}

func NewRPCWriter(ctx context.Context, address string) (*RPCWriter, error) {
	client, err := rpc.DialHTTP("tcp", address)
	if err != nil {
		return nil, err
	}

	rw := &RPCWriter{
		ctx:     ctx,
		address: address,
		client:  client,
	}
	go rw.watchCtx()
	return rw, nil
}

// Sends Measurement Message to RPC Sink
func (rw *RPCWriter) Write(msgs []metrics.MeasurementEnvelope) error {
	if rw.ctx.Err() != nil {
		return rw.ctx.Err()
	}
	if len(msgs) == 0 {
		return nil
	}
	l := log.GetLogger(rw.ctx).
		WithField("sink", "rpc").
		WithField("address", rw.address)
	for _, msg := range msgs {
		var logMsg string
		if err := rw.client.Call("Receiver.UpdateMeasurements", &msg, &logMsg); err != nil {
			return err
		}
		if len(logMsg) > 0 {
			l.Info(logMsg)
		}
	}
	return nil
}

type SyncReq struct {
	DbName     string
	MetricName string
	Operation  string
}

func (rw *RPCWriter) SyncMetric(dbUnique string, metricName string, op string) error {
	var logMsg string
	if err := rw.client.Call("Receiver.SyncMetric", &SyncReq{
		Operation:  op,
		DbName:     dbUnique,
		MetricName: metricName,
	}, &logMsg); err != nil {
		return err
	}
	if len(logMsg) > 0 {
		log.GetLogger(rw.ctx).
			WithField("sink", "rpc").
			WithField("address", rw.address).Info(logMsg)
	}
	return nil
}

func (rw *RPCWriter) watchCtx() {
	<-rw.ctx.Done()
	rw.client.Close()
}
