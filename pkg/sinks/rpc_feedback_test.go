package sinks_test

import (
	"context"
	"net"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v6/api/pb"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/testutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// legacyReceiver stands in for a gRPC server built against the pre-change
// .proto: it serves the three original methods and inherits an Unimplemented
// GetLastMeasurement from pb.UnimplementedReceiverServer, so the Unimplemented
// status comes from gRPC itself rather than from a hand-written stub.
type legacyReceiver struct {
	testutil.Receiver
}

// feedbackReceiver answers GetLastMeasurement with a scripted epoch or status.
type feedbackReceiver struct {
	testutil.Receiver
	epoch int64
	code  codes.Code
}

func (r *feedbackReceiver) GetLastMeasurement(_ context.Context, req *pb.FeedbackReq) (*pb.FeedbackReply, error) {
	if r.code != codes.OK {
		return nil, status.Errorf(r.code, "scripted %s for %s/%s", r.code, req.GetDBName(), req.GetMetricName())
	}
	return &pb.FeedbackReply{EpochNs: r.epoch}, nil
}

// startReceiver serves srv on an ephemeral local port behind the same auth
// interceptor the shared test servers use, so a feedback call that dropped the
// credential metadata would be rejected. It returns the sink connection string.
func startReceiver(t *testing.T, srv pb.ReceiverServer) string {
	t.Helper()
	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	server := grpc.NewServer(grpc.UnaryInterceptor(testutil.AuthInterceptor))
	pb.RegisterReceiverServer(server, srv)
	go func() { _ = server.Serve(lis) }()
	t.Cleanup(server.Stop)

	return "grpc://" + lis.Addr().String()
}
