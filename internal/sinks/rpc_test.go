package sinks_test

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/rpc"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v3/api/pb"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sinks"
	"github.com/stretchr/testify/assert"
)

type Receiver struct {}

var ctxt = context.Background()

const CA = "./rpc_tests_certs/ca.crt"
const ServerCert = "./rpc_tests_certs/server.crt"
const ServerKey = "./rpc_tests_certs/server.key"

var ClientTLSConnStr = fmt.Sprintf("rpc://localhost:5050?sslrootca=%s", CA) // the CN in server test cert is set to `localhost`
const TLSServerAddress = "localhost:5050"

const ServerAddress = "localhost:6060"
const ClientConnStr = "rpc://localhost:6060"

var ConnStrs = [2]string{ClientConnStr, ClientTLSConnStr}

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

func (receiver *Receiver) SyncMetric(syncReq *pb.SyncReq, logMsg *string) error {
	if syncReq == nil {
		return errors.New("msgs is nil")
	}
	if syncReq.Operation == pb.SyncOp_InvalidOp {
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

	rpc.HandleHTTP()
	if listener, err := net.Listen("tcp", ServerAddress); err == nil {
		go func() {
			_ = http.Serve(listener, nil)
		}()
	} else {
		panic(err)
	}	
	
	cert, err := tls.LoadX509KeyPair(ServerCert, ServerKey)
	if err != nil {
		panic(err) 
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	TLSListener, err := tls.Listen("tcp", TLSServerAddress, tlsConfig) 
	if err != nil {
		panic(err)
	}

	go func() {
		for {
			conn, err := TLSListener.Accept()
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
	_, err := sinks.NewRPCWriter(ctxt, ClientConnStr)
	a.NoError(err)

	BadRPCParams := [...]string{"?sslrootca=file.txt", "?sslrootca="}
	for _, value := range BadRPCParams {
		_, err = sinks.NewRPCWriter(ctxt, fmt.Sprintf("localhost:5050%s", value))
		a.Error(err)
	}
}

func TestNewRPCWriter(t *testing.T) {
	a := assert.New(t)
	_, err := sinks.NewRPCWriter(ctxt, "foo")
	a.Error(err)
}

func TestRPCWrite(t *testing.T) {
	a := assert.New(t)

	for _, ConnStr := range ConnStrs {
		rw, err := sinks.NewRPCWriter(ctxt, ConnStr)
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
		rw, err = sinks.NewRPCWriter(ctx, ConnStr)
		a.NoError(err)
		cancel()
		err = rw.Write(msgs)
		a.Error(err)
	}	
}

func TestRPCSyncMetric(t *testing.T) {
	a := assert.New(t)
	for _, ConnStr := range ConnStrs {
		rw, err := sinks.NewRPCWriter(ctxt, ConnStr)
		if err != nil {
			t.Error("Unable to send sync metric signal")
		}

		// no error for valid messages
		err = rw.SyncMetric("Test-DB", "DB-Metric", pb.SyncOp_AddOp)
		a.NoError(err)

		// error for invalid messages
		err = rw.SyncMetric("", "", pb.SyncOp_InvalidOp)
		a.Error(err)

		// error for cancelled context
		ctx, cancel := context.WithCancel(ctxt)
		rw, err = sinks.NewRPCWriter(ctx, ConnStr)
		a.NoError(err)
		cancel()
		err = rw.SyncMetric("Test-DB", "DB-Metric", pb.SyncOp_AddOp)
		a.Error(err)
	}
}