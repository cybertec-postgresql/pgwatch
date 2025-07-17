package sinks

import (
	"context"

	"github.com/cybertec-postgresql/pgwatch/v3/api/pb"
	"google.golang.org/grpc"
)

// RPCWriter is a sink that sends metric measurements to a remote server using the RPC protocol.
// Remote server should implement the Receiver interface. It's up to the implementer to define the
// behavior of the server. It can be a simple logger, external storage, alerting system,
// or an analytics system.
type RPCWriter struct {
	ctx     context.Context
	conn    *grpc.ClientConn
	client  pb.ReceiverClient
}