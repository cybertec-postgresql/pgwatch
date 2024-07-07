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

type Receiver struct {
}

var ctxt = context.Background()

func (receiver *Receiver) UpdateMeasurements(msg *metrics.MeasurementMessage, status *int) error {
	if msg == nil {
		return errors.New("msgs is nil")
	}
	if msg.DBName != "Db" {
		return errors.New("invalid message")
	}
	*status = 1
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

func (receiver *Receiver) SyncMetricSignal(syncReq *sinks.SyncReq, logMsg *string) error {
	*logMsg = "Received>> DBName: " + syncReq.DBName + " OPR: " + syncReq.OPR + " ON: " + syncReq.PgwatchID
	return nil
}

// Test begin from here ---------------------------------------------------------
func TestNewRPCWriter(t *testing.T) {
	a := assert.New(t)
	_, err := sinks.NewRPCWriter(ctxt, "foo")
	a.Error(err)
}

func TestRPCWrite(t *testing.T) {
	a := assert.New(t)
	rw, err := sinks.NewRPCWriter(ctxt, "0.0.0.0:5050")
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

	// error for cancelled context
	ctx, cancel := context.WithCancel(ctxt)
	rw, err = sinks.NewRPCWriter(ctx, "0.0.0.0:5050")
	a.NoError(err)
	cancel()
	err = rw.Write(msgs)
	a.Error(err)
}

func TestRPCSyncMetric(t *testing.T) {
	port := 5050
	a := assert.New(t)
	rw, err := sinks.NewRPCWriter(ctxt, "0.0.0.0:"+fmt.Sprint(port))
	if err != nil {
		t.Error("Unable to send sync metric signal")
	}

	err = rw.SyncMetric("Test-DB", "DB-Metric", "Add")
	a.NoError(err)
}
