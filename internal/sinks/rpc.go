package sinks

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/rpc"
	"os"
	"strings"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
)

func NewRPCWriter(ctx context.Context, path string) (*RPCWriter, error) {
	address, params, found := strings.Cut(path, "?")
	if (!found) {
		return nil, fmt.Errorf("invalid RPC connection string, Usage: rpc://host?sslrootca=[CA_file]")
	}

	key, RootCA, found := strings.Cut(params, "=")
	if (!found || key != "sslrootca") {
		return nil, fmt.Errorf("invalid RPC parameter %s", key)
	}

	ca, err := os.ReadFile(RootCA)
	if err != nil {
		return nil, fmt.Errorf("cannot load CA file: %s", err)
	}

	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(ca)

	tlsClientConfig := &tls.Config{
		RootCAs: certPool,
	}

	conn, err := tls.Dial("tcp", address, tlsClientConfig)

	if err != nil {
		return nil, err
	}

	l := log.GetLogger(ctx).WithField("sink", "rpc").WithField("address", address)
	ctx = log.WithLogger(ctx, l)
	rw := &RPCWriter{
		ctx:     ctx,
		address: address,
		client:  rpc.NewClient(conn),
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
