package sinks_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/rpc"
	"testing"

	"github.com/cybertec-postgresql/pgwatch3/metrics"
	"github.com/cybertec-postgresql/pgwatch3/sinks"
	"github.com/stretchr/testify/assert"
)

type RPCWriter struct {
}

var ctxt = context.Background()

func (receiver *RPCWriter) UpdateMeasurements(msgs []metrics.MeasurementMessage, status *int) error {
	if msgs == nil {
		return errors.New("msgs is nil")
	}
	if len(msgs) == 0 {
		return nil
	}
	if msgs[0].DBName != "Db" {
		return errors.New("invalid message")
	}
	*status = 1
	return nil
}

func init() {
	recv := new(RPCWriter)
	rpc.Register(recv)
	rpc.HandleHTTP()

	listener, err := net.Listen("tcp", "0.0.0.0:5050")
	if err != nil {
		panic(err)
	}
	go http.Serve(listener, nil)
}

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
