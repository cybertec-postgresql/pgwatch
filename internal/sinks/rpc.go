package sinks

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/rpc"
	"os"
	"net/url"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
)

func NewRPCWriter(ctx context.Context, ConnStr string) (*RPCWriter, error) {
	uri, err := url.Parse(ConnStr)
	if err != nil {
		return nil, fmt.Errorf("error parsing RPC URI: %s", err)
	}

	params, err := url.ParseQuery(uri.RawQuery)
	if err != nil {
		return nil, fmt.Errorf("error parsing RPC URI parameters: %s", err)
	}

	RootCA, exists := params["sslrootca"]
	var client *rpc.Client
	if exists {
		client, err = connectViaTLS(uri.Host, RootCA[0])
	} else {
		client, err = rpc.DialHTTP("tcp", uri.Host)
	}

	if err != nil {
		return nil, err
	}

	l := log.GetLogger(ctx).WithField("sink", "rpc").WithField("address", uri.Host)
	ctx = log.WithLogger(ctx, l)
	rw := &RPCWriter{
		ctx:     ctx,
		client:  client,
	}
	go rw.watchCtx()
	return rw, nil
}

func connectViaTLS(address, RootCA string) (*rpc.Client, error) {
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
	return rpc.NewClient(conn), nil
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
