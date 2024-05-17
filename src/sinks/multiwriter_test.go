package sinks

import (
	"context"
	"testing"

	"github.com/cybertec-postgresql/pgwatch3/config"
	"github.com/cybertec-postgresql/pgwatch3/metrics"
	"github.com/stretchr/testify/assert"
)

type MockWriter struct{}

func (mw *MockWriter) SyncMetric(_, _, _ string) error {
	return nil
}

func (mw *MockWriter) Write(_ []metrics.MeasurementMessage) error {
	return nil
}

func TestNewMultiWriter(t *testing.T) {
	input := []struct {
		opts *config.Options
		mw   bool // MultiWriter returned
		err  bool // error returned
	}{
		{&config.Options{}, false, true},
		{&config.Options{
			Measurements: config.MeasurementOpts{
				Sinks: []string{"foo"},
			},
		}, false, true},
		{&config.Options{
			Measurements: config.MeasurementOpts{
				Sinks: []string{"jsonfile://test.json"},
			},
		}, true, false},
	}

	for _, i := range input {
		mw, err := NewMultiWriter(context.Background(), i.opts, *metrics.GetDefaultMetrics())
		if i.err {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
		if i.mw {
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
	err := mw.SyncMetrics("db", "metric", "op")
	assert.NoError(t, err)
}

func TestWriteMeasurements(t *testing.T) {
	mw := &MultiWriter{}
	mockWriter := &MockWriter{}
	mw.AddWriter(mockWriter)
	ctx, c := context.WithCancel(context.Background())
	assert.NotNil(t, c)
	storageCh := make(chan []metrics.MeasurementMessage)
	go mw.WriteMeasurements(ctx, storageCh)
	storageCh <- []metrics.MeasurementMessage{{}}
	c()
	close(storageCh)
}
