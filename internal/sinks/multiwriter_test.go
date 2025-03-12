package sinks

import (
	"context"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/stretchr/testify/assert"
)

type MockWriter struct{}

func (mw *MockWriter) SyncMetric(_, _, _ string) error {
	return nil
}

func (mw *MockWriter) Write(_ []metrics.MeasurementEnvelope) error {
	return nil
}

func TestNewMultiWriter(t *testing.T) {
	input := []struct {
		opts *CmdOpts
		w    bool // Writer returned
		err  bool // error returned
	}{
		{&CmdOpts{}, false, true},
		{&CmdOpts{
			Sinks: []string{"foo"},
		}, false, true},
		{&CmdOpts{
			Sinks: []string{"jsonfile://test.json"},
		}, true, false},
		{&CmdOpts{
			Sinks: []string{"jsonfile://test.json", "jsonfile://test1.json"},
		}, true, false},
		{&CmdOpts{
			Sinks: []string{"prometheus://foo/"},
		}, false, true},
		{&CmdOpts{
			Sinks: []string{"rpc://foo/"},
		}, false, true},
		{&CmdOpts{
			Sinks: []string{"postgresql:///baz"},
		}, false, true},
		{&CmdOpts{
			Sinks: []string{"foo:///"},
		}, false, true},
	}

	for _, i := range input {
		mw, err := NewSinkWriter(context.Background(), i.opts, metrics.GetDefaultMetrics())
		if i.err {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
		if i.w {
			assert.NotNil(t, mw)
		} else {
			assert.Nil(t, mw)
		}
	}
}

func TestAddWriter(t *testing.T) {
	mw := &MultiWriter{}
	mockWriter := &MockWriter{}
	mw.AddWriter(mockWriter)
	assert.Equal(t, 1, len(mw.writers))
}

func TestSyncMetrics(t *testing.T) {
	mw := &MultiWriter{}
	mockWriter := &MockWriter{}
	mw.AddWriter(mockWriter)
	err := mw.SyncMetric("db", "metric", "op")
	assert.NoError(t, err)
}

func TestWriteMeasurements(t *testing.T) {
	mw := &MultiWriter{}
	mockWriter := &MockWriter{}
	mw.AddWriter(mockWriter)
	err := mw.Write([]metrics.MeasurementEnvelope{{}})
	assert.NoError(t, err)
}
