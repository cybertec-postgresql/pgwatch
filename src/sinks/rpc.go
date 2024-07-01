/*
*
* RPC sink implementation for pgwatch3.
* Requires the address and port of the sink.
*
 */

package sinks

import (
	"context"
	"net/rpc"

	"github.com/cybertec-postgresql/pgwatch3/metrics"
)

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
func (rw *RPCWriter) Write(msgs []metrics.MeasurementMessage) error {
	if rw.ctx.Err() != nil {
		return rw.ctx.Err()
	}
	if len(msgs) == 0 {
		return nil
	}
	var status int
	err := rw.client.Call("Receiver.UpdateMeasurements", msgs, &status)
	if err != nil {
		return err
	}
	return nil
}

func (rw *RPCWriter) SyncMetric(_, _, _ string) error {
	if rw.ctx.Err() != nil {
		return rw.ctx.Err()
	}
	return nil
}

func (rw *RPCWriter) watchCtx() {
	<-rw.ctx.Done()
	rw.client.Close()
}
