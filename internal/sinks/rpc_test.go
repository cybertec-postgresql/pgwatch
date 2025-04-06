package sinks_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/rpc"
	"testing"

	"github.com/cybertec-postgresql/pgwatch3/metrics"
	"github.com/cybertec-postgresql/pgwatch3/sinks"
	"github.com/stretchr/testify/assert"
)

const validToken = "test_token"

type Receiver struct {
}

var ctxt = context.Background()

func (receiver *Receiver) UpdateMeasurements(req *sinks.AuthRequest, logMsg *string) error {
	if req.Token != validToken {
		return errors.New("invalid token")
	}

	msg, ok := req.Data.(metrics.MeasurementMessage)
	if !ok {
		return errors.New("invalid data format")
	}

	if msg.DBName != "Db" {
		return errors.New("invalid message")
	}

	*logMsg = fmt.Sprintf("Received: %+v", msg)
	return nil
}

func (receiver *Receiver) SyncMetric(req *sinks.AuthRequest, logMsg *string) error {
	if req.Token != validToken {
		return errors.New("invalid token")
	}

	syncReq, ok := req.Data.(sinks.SyncReq)
	if !ok {
		return errors.New("invalid data format")
	}

	if syncReq.Operation == "invalid" {
		return errors.New("invalid message")
	}

	*logMsg = fmt.Sprintf("Received: %+v", syncReq)
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
	_, err := sinks.NewRPCWriter(ctxt, "foo")
	a.Error(err)
}

func TestRPCWrite(t *testing.T) {
	a := assert.New(t)

	// Test with valid token
	rw, err := sinks.NewRPCWriter(ctxt, validToken+"@0.0.0.0:5050")
	a.NoError(err)

	// no error for valid messages
	msgs := []metrics.MeasurementMessage{
		{
			DBName: "Db",
		},
	}
	err = rw.Write(msgs)
	a.NoError(err)

	// error for invalid messages
	msgs = []metrics.MeasurementMessage{
		{
			DBName: "invalid",
		},
	}
	err = rw.Write(msgs)
	a.Error(err)

	// no error for empty messages
	err = rw.Write([]metrics.MeasurementMessage{})
	a.NoError(err)

	// Test with invalid token
	rw, err = sinks.NewRPCWriter(ctxt, "invalid_token@0.0.0.0:5050")
	a.NoError(err)

	err = rw.Write(msgs)
	a.Error(err)

	// error for cancelled context
	ctx, cancel := context.WithCancel(ctxt)
	rw, err = sinks.NewRPCWriter(ctx, validToken+"@0.0.0.0:5050")
	a.NoError(err)
	cancel()
	err = rw.Write(msgs)
	a.Error(err)
}

func TestRPCSyncMetric(t *testing.T) {
	port := 5050
	a := assert.New(t)

	// Test with valid token
	rw, err := sinks.NewRPCWriter(ctxt, validToken+"@0.0.0.0:"+fmt.Sprint(port))
	if err != nil {
		t.Error("Unable to send sync metric signal")
	}

	// no error for valid messages
	err = rw.SyncMetric("Test-DB", "DB-Metric", "Add")
	a.NoError(err)

	// error for invalid messages
	err = rw.SyncMetric("", "", "invalid")
	a.Error(err)

	// Test with invalid token
	rw, err = sinks.NewRPCWriter(ctxt, "invalid_token@0.0.0.0:"+fmt.Sprint(port))
	a.NoError(err)

	err = rw.SyncMetric("Test-DB", "DB-Metric", "Add")
	a.Error(err)

	// error for cancelled context
	ctx, cancel := context.WithCancel(ctxt)
	rw, err = sinks.NewRPCWriter(ctx, validToken+"@0.0.0.0:5050")
	a.NoError(err)
	cancel()
	err = rw.SyncMetric("Test-DB", "DB-Metric", "Add")
	a.Error(err)
}
