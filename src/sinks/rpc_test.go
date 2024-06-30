package sinks

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/rpc"
	"testing"

	"github.com/cybertec-postgresql/pgwatch3/metrics"
)

type Receiver struct {
}

var ctxt = context.Background()

func (receiver *Receiver) UpdateMeasurements(msg *metrics.MeasurementMessage, status *int) error {
	*status = 1
	return nil
}

func setupServer(port int) error {
	recv := new(Receiver)
	rpc.Register(recv)
	rpc.HandleHTTP()

	listener, err := net.Listen("tcp", "0.0.0.0:"+fmt.Sprint(port))
	if err != nil {
		return err
	}
	go http.Serve(listener, nil)
	return nil
}

func TestNewRPCWriter(t *testing.T) {
	port := 5050
	setupServer(port)
	_, err := NewRPCWriter(ctxt, "0.0.0.0:"+fmt.Sprint(port))
	if err != nil {
		t.Log("Unable to create new RPC client, Error: ", err)
		t.Failed()
	}
}

func TestRPCWrite(t *testing.T) {
	port := 5050
	setupServer(port)
	rw, err := NewRPCWriter(ctxt, "0.0.0.0:"+fmt.Sprint(port))
	if err != nil {
		t.Error("Unable to create new RPC client, Error: ", err)
	}

	msgs := []metrics.MeasurementMessage{
		metrics.MeasurementMessage{
			DBName: "Db",
		},
		metrics.MeasurementMessage{
			DBName: "Db2",
		},
	}

	err = rw.Write(msgs)
	if err != nil {
		t.Error("Unable to Write Messages to sink, Error: ", err)
	}
}
