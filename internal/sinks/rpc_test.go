package sinks

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/rpc"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/stretchr/testify/assert"
)

type Receiver struct {
}

var ctxt = context.Background()

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

func (receiver *Receiver) SyncMetric(syncReq *SyncReq, logMsg *string) error {
	if syncReq == nil {
		return errors.New("msgs is nil")
	}
	if syncReq.Operation == invalidOp {
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
	if listener, err := net.Listen("tcp", "0.0.0.0:5050"); err == nil {
		go func() {
			_ = http.Serve(listener, nil)
		}()
	} else {
		panic(err)
	}
}

// Test begin from here ---------------------------------------------------------
func TestNewRPCWriter(t *testing.T) {
	a := assert.New(t)
	_, err := NewRPCWriter(ctxt, "foo")
	a.Error(err)
}

func TestRPCWrite(t *testing.T) {
	a := assert.New(t)
	rw, err := NewRPCWriter(ctxt, "0.0.0.0:5050")
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
	rw, err = NewRPCWriter(ctx, "0.0.0.0:5050")
	a.NoError(err)
	cancel()
	err = rw.Write(msgs)
	a.Error(err)
}

func TestRPCSyncMetric(t *testing.T) {
	port := 5050
	a := assert.New(t)
	rw, err := NewRPCWriter(ctxt, "0.0.0.0:"+fmt.Sprint(port))
	if err != nil {
		t.Error("Unable to send sync metric signal")
	}

	// no error for valid messages
	err = rw.SyncMetric("Test-DB", "DB-Metric", AddOp)
	a.NoError(err)

	// error for invalid messages
	err = rw.SyncMetric("", "", invalidOp)
	a.Error(err)

	// error for cancelled context
	ctx, cancel := context.WithCancel(ctxt)
	rw, err = NewRPCWriter(ctx, "0.0.0.0:5050")
	a.NoError(err)
	cancel()
	err = rw.SyncMetric("Test-DB", "DB-Metric", AddOp)
	a.Error(err)
}
