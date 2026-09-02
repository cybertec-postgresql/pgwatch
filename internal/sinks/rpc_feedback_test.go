package sinks_test

import (
	"context"
	"net"
	"sync"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v6/api/pb"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/sinks"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/testutil"
	"github.com/stretchr/testify/assert"
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

// TestRPCLastMeasurementStatusMapping walks the status-code contract in
// spec/design-sink-feedback.md §4.5.
func TestRPCLastMeasurementStatusMapping(t *testing.T) {
	for _, tc := range []struct {
		name      string
		recv      *feedbackReceiver
		wantEpoch int64
		wantErr   error
		wantCode  codes.Code
	}{
		{
			name:      "ok with positive epoch",
			recv:      &feedbackReceiver{epoch: 1756800000000000000},
			wantEpoch: 1756800000000000000,
		},
		{
			name:    "ok with zero epoch",
			recv:    &feedbackReceiver{epoch: 0},
			wantErr: sinks.ErrNoFeedbackData,
		},
		{
			name:    "ok with negative epoch",
			recv:    &feedbackReceiver{epoch: -1},
			wantErr: sinks.ErrNoFeedbackData,
		},
		{
			name:    "not found",
			recv:    &feedbackReceiver{code: codes.NotFound},
			wantErr: sinks.ErrNoFeedbackData,
		},
		{
			name:    "unimplemented",
			recv:    &feedbackReceiver{code: codes.Unimplemented},
			wantErr: sinks.ErrFeedbackUnsupported,
		},
		{
			name:     "unavailable propagates",
			recv:     &feedbackReceiver{code: codes.Unavailable},
			wantCode: codes.Unavailable,
		},
		{
			name:     "internal propagates",
			recv:     &feedbackReceiver{code: codes.Internal},
			wantCode: codes.Internal,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rw, err := sinks.NewRPCWriter(ctx, startReceiver(t, tc.recv), &sinks.CmdOpts{})
			require.NoError(t, err)

			epoch, err := rw.LastMeasurement(ctx, "prod-db", "db_stats")
			switch {
			case tc.wantErr != nil:
				assert.ErrorIs(t, err, tc.wantErr)
				assert.Zero(t, epoch)
			case tc.wantCode != codes.OK:
				assert.Equal(t, tc.wantCode, status.Code(err))
				assert.Zero(t, epoch)
			default:
				require.NoError(t, err)
				assert.Equal(t, tc.wantEpoch, epoch)
			}
		})
	}
}

// TestRPCTransientErrorKeepsCapability pins that only Unimplemented latches the
// capability off; a server that is merely down must be retried later.
func TestRPCTransientErrorKeepsCapability(t *testing.T) {
	rw, err := sinks.NewRPCWriter(ctx, startReceiver(t, &feedbackReceiver{code: codes.Unavailable}), &sinks.CmdOpts{})
	require.NoError(t, err)

	_, err = rw.LastMeasurement(ctx, "prod-db", "db_stats")
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.True(t, rw.CanFeedback("prod-db", "db_stats"), "a transient failure must not latch the capability off")
}

func TestRPCCanFeedback(t *testing.T) {
	connStr := startReceiver(t, &feedbackReceiver{epoch: 1})

	rw, err := sinks.NewRPCWriter(ctx, connStr, &sinks.CmdOpts{})
	require.NoError(t, err)
	assert.True(t, rw.CanFeedback("prod-db", "db_stats"))
	assert.False(t, rw.CanFeedback("", "db_stats"))
	assert.False(t, rw.CanFeedback("prod-db", ""))

	disabled, err := sinks.NewRPCWriter(ctx, connStr, &sinks.CmdOpts{NoFeedback: true})
	require.NoError(t, err)
	assert.False(t, disabled.CanFeedback("prod-db", "db_stats"))

	epoch, err := disabled.LastMeasurement(ctx, "prod-db", "db_stats")
	assert.ErrorIs(t, err, sinks.ErrFeedbackUnsupported)
	assert.Zero(t, epoch)
}

// TestRPCLegacyReceiverLatchesOff pins RPC-003 against a receiver that never
// heard of GetLastMeasurement: gRPC itself answers Unimplemented, the sink
// maps that to ErrFeedbackUnsupported, and it never asks again.
func TestRPCLegacyReceiverLatchesOff(t *testing.T) {
	recv := &legacyReceiver{}
	rw, err := sinks.NewRPCWriter(ctx, startReceiver(t, recv), &sinks.CmdOpts{})
	require.NoError(t, err)
	require.True(t, rw.CanFeedback("prod-db", "db_stats"), "optimistic before the first probe")

	epoch, err := rw.LastMeasurement(ctx, "prod-db", "db_stats")
	assert.ErrorIs(t, err, sinks.ErrFeedbackUnsupported)
	assert.Zero(t, epoch)

	// The capability is latched off for every pair, without a round-trip.
	assert.False(t, rw.CanFeedback("prod-db", "db_stats"))
	assert.False(t, rw.CanFeedback("other-db", "other_metric"))

	_, err = rw.LastMeasurement(ctx, "other-db", "other_metric")
	assert.ErrorIs(t, err, sinks.ErrFeedbackUnsupported)
}

// TestRPCFeedbackRace hammers the latched capability flag from several
// goroutines. Run under -race.
func TestRPCFeedbackRace(t *testing.T) {
	rw, err := sinks.NewRPCWriter(ctx, startReceiver(t, &legacyReceiver{}), &sinks.CmdOpts{})
	require.NoError(t, err)

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 25 {
				_, _ = rw.LastMeasurement(ctx, "prod-db", "db_stats")
				rw.CanFeedback("prod-db", "db_stats")
			}
		})
	}
	wg.Wait()

	assert.False(t, rw.CanFeedback("prod-db", "db_stats"), "the latch must settle off")
}
