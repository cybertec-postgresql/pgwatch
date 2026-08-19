package testutil

import (
	"context"
	"errors"

	"github.com/cybertec-postgresql/pgwatch/v6/api/pb"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/db"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/sources"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/structpb"
)

// Receiver implements the ReceiverServer interface for testing purposes
type Receiver struct {
	pb.UnimplementedReceiverServer
}

func (receiver *Receiver) UpdateMeasurements(_ context.Context, msg *pb.MeasurementEnvelope) (*pb.Reply, error) {
	if len(msg.GetData()) == 0 {
		return nil, errors.New("empty message")
	}
	if msg.GetDBName() != "Db" {
		return nil, errors.New("invalid message")
	}
	return &pb.Reply{}, nil
}

func (receiver *Receiver) SyncMetric(_ context.Context, syncReq *pb.SyncReq) (*pb.Reply, error) {
	if syncReq == nil {
		return nil, errors.New("nil sync request")
	}
	if syncReq.GetOperation() == pb.SyncOp_InvalidOp {
		return nil, errors.New("invalid sync request")
	}
	return &pb.Reply{}, nil
}

func (receiver *Receiver) DefineMetrics(_ context.Context, metricsStruct *structpb.Struct) (*pb.Reply, error) {
	if metricsStruct == nil {
		return nil, errors.New("nil metrics struct")
	}
	if metricsStruct.GetFields() == nil {
		return nil, errors.New("empty metrics struct")
	}
	return &pb.Reply{Logmsg: "metrics defined successfully"}, nil
}

// MockMetricsReaderWriter implements MetricsReaderWriter interface
type MockMetricsReaderWriter struct {
	GetMetricsFunc   func() (*metrics.Metrics, error)
	UpdateMetricFunc func(name string, m metrics.Metric) error
	CreateMetricFunc func(name string, m metrics.Metric) error
	DeleteMetricFunc func(name string) error
	DeletePresetFunc func(name string) error
	UpdatePresetFunc func(name string, preset metrics.Preset) error
	CreatePresetFunc func(name string, preset metrics.Preset) error
	WriteMetricsFunc func(metricDefs *metrics.Metrics) error
}

func (m *MockMetricsReaderWriter) GetMetrics() (*metrics.Metrics, error) {
	return m.GetMetricsFunc()
}
func (m *MockMetricsReaderWriter) UpdateMetric(name string, metric metrics.Metric) error {
	return m.UpdateMetricFunc(name, metric)
}
func (m *MockMetricsReaderWriter) CreateMetric(name string, metric metrics.Metric) error {
	return m.CreateMetricFunc(name, metric)
}
func (m *MockMetricsReaderWriter) DeleteMetric(name string) error {
	return m.DeleteMetricFunc(name)
}
func (m *MockMetricsReaderWriter) DeletePreset(name string) error {
	return m.DeletePresetFunc(name)
}
func (m *MockMetricsReaderWriter) UpdatePreset(name string, preset metrics.Preset) error {
	return m.UpdatePresetFunc(name, preset)
}
func (m *MockMetricsReaderWriter) CreatePreset(name string, preset metrics.Preset) error {
	return m.CreatePresetFunc(name, preset)
}
func (m *MockMetricsReaderWriter) WriteMetrics(metricDefs *metrics.Metrics) error {
	return m.WriteMetricsFunc(metricDefs)
}

// MockSourcesReaderWriter implements SourcesReaderWriter interface
type MockSourcesReaderWriter struct {
	GetSourcesFunc   func() (sources.Sources, error)
	UpdateSourceFunc func(md sources.Source) error
	CreateSourceFunc func(md sources.Source) error
	DeleteSourceFunc func(name string) error
	WriteSourcesFunc func(sources.Sources) error
}

func (m *MockSourcesReaderWriter) GetSources() (sources.Sources, error) {
	return m.GetSourcesFunc()
}
func (m *MockSourcesReaderWriter) UpdateSource(md sources.Source) error {
	return m.UpdateSourceFunc(md)
}
func (m *MockSourcesReaderWriter) CreateSource(md sources.Source) error {
	return m.CreateSourceFunc(md)
}
func (m *MockSourcesReaderWriter) DeleteSource(name string) error {
	return m.DeleteSourceFunc(name)
}
func (m *MockSourcesReaderWriter) WriteSources(srcs sources.Sources) error {
	return m.WriteSourcesFunc(srcs)
}


// BlockingPool is a wedged PgxPoolIface used by fault-injection tests.
//
// It embeds db.PgxPoolIface as a nil interface so the type satisfies
// db.PgxPoolIface; only the methods overridden below are usable. Any
// direct call to a non-overridden method panics (nil interface call).
//
// Ping, Query, SendBatch, and Acquire all block until the supplied
// context is cancelled, then return ctx.Err(). This simulates a pool
// whose connections are stuck on the wire — the failure mode that
// client-side deadlines must convert into bounded failures.
type BlockingPool struct {
	db.PgxPoolIface
}

// Ping blocks until ctx.Done() and returns ctx.Err().
func (BlockingPool) Ping(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

// Query blocks until ctx.Done() and returns ctx.Err().
func (BlockingPool) Query(ctx context.Context, _ string, _ ...any) (pgx.Rows, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// QueryRow blocks until ctx.Done() and returns a row whose Scan returns ctx.Err().
func (BlockingPool) QueryRow(ctx context.Context, _ string, _ ...any) pgx.Row {
	return blockingRow{ctx: ctx}
}

// Exec blocks until ctx.Done() and returns ctx.Err().
func (BlockingPool) Exec(ctx context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	<-ctx.Done()
	return pgconn.CommandTag{}, ctx.Err()
}
// first Query call returns ctx.Err().
// SendBatch blocks until ctx.Done() and returns a BatchResults whose
// first Query call returns ctx.Err().
func (BlockingPool) SendBatch(ctx context.Context, _ *pgx.Batch) pgx.BatchResults {
	return &BlockingBatchResults{ctx: ctx}
}

// Acquire blocks until ctx.Done() and returns ctx.Err().
func (BlockingPool) Acquire(ctx context.Context) (*pgxpool.Conn, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// Close is a no-op so the pool can be embedded without panicking.
func (BlockingPool) Close() {}

// blockingRow makes pgx.Row.Scan honor ctx cancellation.
type blockingRow struct {
	ctx context.Context
}

// Scan blocks until ctx.Done() and returns ctx.Err().
func (b blockingRow) Scan(_ ...any) error {
	<-b.ctx.Done()
	return b.ctx.Err()
}

// BlockingBatchResults makes the first Query call honor ctx cancellation.
type BlockingBatchResults struct {
	ctx    context.Context
	closed bool
}

// Query blocks until ctx.Done() and returns ctx.Err().
func (b *BlockingBatchResults) Query() (pgx.Rows, error) {
	<-b.ctx.Done()
	return nil, b.ctx.Err()
}

// Exec blocks until ctx.Done() and returns ctx.Err().
func (b *BlockingBatchResults) Exec() (pgconn.CommandTag, error) {
	<-b.ctx.Done()
	return pgconn.CommandTag{}, b.ctx.Err()
}
func (b *BlockingBatchResults) Close() error {
	b.closed = true
	return nil
}

// Err returns ctx.Err() if the batch was closed via Close.

// QueryRow blocks until ctx.Done() and returns a Row whose Scan returns ctx.Err().
func (b *BlockingBatchResults) QueryRow() pgx.Row {
	return blockingRow{ctx: b.ctx}
}
func (b *BlockingBatchResults) Err() error {
	if b.closed {
		return b.ctx.Err()
	}
	return nil
}