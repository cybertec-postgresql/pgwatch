package sinks

import (
	"context"
	"net/rpc"
)

// RPCWriter is a sink that sends metric measurements to a remote server using the RPC protocol.
// Remote server should implement the Receiver interface. It's up to the implementer to define the
// behavior of the server. It can be a simple logger, external storage, alerting system,
// or an analytics system.
type RPCWriter struct {
	ctx     context.Context
	client  *rpc.Client
}

type SyncOp int

const (
	AddOp SyncOp = iota
	DeleteOp
	InvalidOp
)

type SyncReq struct {
	DbName     string
	MetricName string
	Operation  SyncOp
}
