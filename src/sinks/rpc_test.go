package sinks

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/rpc"
	"testing"

	"github.com/cybertec-postgresql/pgwatch3/metrics"
	"github.com/stretchr/testify/assert"
)

type Receiver struct {
}

var ctxt = context.Background()

func (receiver *Receiver) UpdateMeasurements(msgs []metrics.MeasurementMessage, status *int) error {
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
	recv := new(Receiver)
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
	_, err := NewRPCWriter(ctxt, "foo")
	a.Error(err)
}

func TestRPCWrite(t *testing.T) {
	a := assert.New(t)
	rw, err := NewRPCWriter(ctxt, "0.0.0.0:5050")
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
	rw.ctx = ctx
	cancel()
	err = rw.Write(msgs)
	a.Error(err)
}
