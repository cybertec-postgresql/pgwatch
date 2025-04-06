package sinks

import (
	"context"
	"net/rpc"
	"strings"

	"github.com/cybertec-postgresql/pgwatch3/log"
	"github.com/cybertec-postgresql/pgwatch3/metrics"
)

// AuthRequest wraps RPC requests with authentication
type AuthRequest struct {
	Token string      `json:"token"`
	Data  interface{} `json:"data"`
}

type RPCWriter struct {
	ctx     context.Context
	address string
	client  *rpc.Client
	token   string // Added authentication token
}

// extractTokenFromAddress extracts token from rpc://token@host:port format
func extractTokenFromAddress(address string) (string, string) {
	if !strings.Contains(address, "@") {
		return "", address
	}

	parts := strings.Split(address, "@")
	return parts[0], parts[1]
}

func NewRPCWriter(ctx context.Context, address string) (*RPCWriter, error) {
	token, cleanAddress := extractTokenFromAddress(address)

	client, err := rpc.DialHTTP("tcp", cleanAddress)
	if err != nil {
		return nil, err
	}

	rw := &RPCWriter{
		ctx:     ctx,
		address: cleanAddress,
		client:  client,
		token:   token,
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
	l := log.GetLogger(rw.ctx).
		WithField("sink", "rpc").
		WithField("address", rw.address)
	for _, msg := range msgs {
		var logMsg string

		// Wrap message with authentication
		authReq := AuthRequest{
			Token: rw.token,
			Data:  msg,
		}

		if err := rw.client.Call("Receiver.UpdateMeasurements", &authReq, &logMsg); err != nil {
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

	syncReq := SyncReq{
		Operation:  op,
		DbName:     dbUnique,
		MetricName: metricName,
	}

	// Wrap with authentication
	authReq := AuthRequest{
		Token: rw.token,
		Data:  syncReq,
	}

	if err := rw.client.Call("Receiver.SyncMetric", &authReq, &logMsg); err != nil {
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
