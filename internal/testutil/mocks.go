package testutil

import (
	"context"
	"errors"

	"github.com/cybertec-postgresql/pgwatch/v3/api/pb"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
	"google.golang.org/protobuf/types/known/structpb"
)

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

type MockReader struct {
	ToReturn sources.Sources
	ToErr    error
}

func (m *MockReader) GetSources() (sources.Sources, error) {
	if m.ToErr != nil {
		return nil, m.ToErr
	}
	return m.ToReturn, nil
}

func (m *MockReader) WriteSources(sources.Sources) error { return nil }
func (m *MockReader) DeleteSource(string) error          { return nil }
func (m *MockReader) UpdateSource(sources.Source) error  { return nil }
func (m *MockReader) CreateSource(sources.Source) error  { return nil }

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