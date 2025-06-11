package sinks_test

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/rpc"
	"os"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sinks"
	"github.com/stretchr/testify/assert"
)

type Receiver struct {}

var ctxt = context.Background()
var address = "localhost:5050" // the CN in server test cert is set to `localhost`

const CA = "./rpc_tests_certs/ca.crt"
const ServerCert = "./rpc_tests_certs/server.crt"
const ServerKey = "./rpc_tests_certs/server.key"

var opts = &sinks.CmdOpts{RootCA: CA}

func (receiver *Receiver) UpdateMeasurements(msg *metrics.MeasurementEnvelope, logMsg *string) error {
	if msg == nil || len(msg.Data) == 0 {
		return errors.New("msgs is nil")
	}
	if msg.DBName != "Db" {
		return errors.New("invalid message")
	}
	*logMsg = fmt.Sprintf("Received: %+v", *msg)
	return nil
}

func (receiver *Receiver) SyncMetric(syncReq *sinks.SyncReq, logMsg *string) error {
	if syncReq == nil {
		return errors.New("msgs is nil")
	}
	if syncReq.Operation == sinks.InvalidOp {
		return errors.New("invalid message")
	}
	*logMsg = fmt.Sprintf("Received: %+v", *syncReq)
	return nil
}

func init() {
	recv := new(Receiver)
	if err := rpc.Register(recv); err != nil {
		panic(err)
	}
	
	cert, err := tls.LoadX509KeyPair(ServerCert, ServerKey)
	if err != nil {
		return 
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	listener, err := tls.Listen("tcp", address, tlsConfig) 
	if err != nil {
		panic(err)
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				continue
			}
			go rpc.ServeConn(conn)
		}
	}()
}

// Test begin from here ---------------------------------------------------------

func TestCACertValidation(t *testing.T) {
	a := assert.New(t)
	_, err := sinks.NewRPCWriter(ctxt, address, opts)
	a.NoError(err)

	_, err = sinks.NewRPCWriter(ctxt, address, &sinks.CmdOpts{RootCA: ""})
	a.Error(err)

	fileName := "invalid_CA_file.txt"
	_, err = os.Create(fileName)
	a.NoError(err)

	_, err = sinks.NewRPCWriter(ctxt, address, &sinks.CmdOpts{RootCA: fileName})
	a.Error(err)

	os.Remove(fileName)
}

func TestNewRPCWriter(t *testing.T) {
	a := assert.New(t)
	_, err := sinks.NewRPCWriter(ctxt, "foo", opts)
	a.Error(err)
}

func TestRPCWrite(t *testing.T) {
	a := assert.New(t)
	rw, err := sinks.NewRPCWriter(ctxt, address, opts)
	a.NoError(err)

	// no error for valid messages
	msgs := metrics.MeasurementEnvelope{
		DBName: "Db",
		Data:   metrics.Measurements{{"test": 1}},
	}
	err = rw.Write(msgs)
	a.NoError(err)

	// error for invalid messages
	msgs = metrics.MeasurementEnvelope{
		DBName: "invalid",
	}
	err = rw.Write(msgs)
	a.Error(err)

	// error for empty messages
	err = rw.Write(metrics.MeasurementEnvelope{})
	a.Error(err)

	// error for cancelled context
	ctx, cancel := context.WithCancel(ctxt)
	rw, err = sinks.NewRPCWriter(ctx, address, opts)
	a.NoError(err)
	cancel()
	err = rw.Write(msgs)
	a.Error(err)
}

func TestRPCSyncMetric(t *testing.T) {
	a := assert.New(t)
	rw, err := sinks.NewRPCWriter(ctxt, address, opts)
	if err != nil {
		t.Error("Unable to send sync metric signal")
	}

	// no error for valid messages
	err = rw.SyncMetric("Test-DB", "DB-Metric", sinks.AddOp)
	a.NoError(err)

	// error for invalid messages
	err = rw.SyncMetric("", "", sinks.InvalidOp)
	a.Error(err)

	// error for cancelled context
	ctx, cancel := context.WithCancel(ctxt)
	rw, err = sinks.NewRPCWriter(ctx, address, opts)
	a.NoError(err)
	cancel()
	err = rw.SyncMetric("Test-DB", "DB-Metric", sinks.AddOp)
	a.Error(err)
}