package reaper

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
)

// ScrapeAll fetches Prometheus exposition metrics from sc and returns one
// MeasurementEnvelope per metric family. Each sample becomes one Measurement
// with tag_<label> columns (skipping __name__), a value column named after the
// family, and epoch_ns set from the sample timestamp (ms→ns) or time.Now().
func ScrapeAll(ctx context.Context, sc *sources.PromConn) ([]metrics.MeasurementEnvelope, error) {
	resp, err := sc.Scrape(ctx)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("scrapeall: unexpected status %s", resp.Status)
	}

	contentType := expfmt.ResponseFormat(resp.Header)
	decoder := expfmt.NewDecoder(resp.Body, contentType)

	var result []metrics.MeasurementEnvelope
	for {
		var mf dto.MetricFamily
		if err := decoder.Decode(&mf); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("scrapeall: decoding: %w", err)
		}

		familyName := mf.GetName()
		samples := make(metrics.Measurements, 0, len(mf.GetMetric()))
		for _, m := range mf.GetMetric() {
			measurement := make(metrics.Measurement)

			for _, lp := range m.GetLabel() {
				if lp.GetName() != "__name__" {
					measurement[metrics.TagPrefix+lp.GetName()] = lp.GetValue()
				}
			}

			measurement[familyName] = promMetricValue(m, mf.GetType())

			if ts := m.GetTimestampMs(); ts != 0 {
				measurement[metrics.EpochColumnName] = ts * 1_000_000
			} else {
				measurement[metrics.EpochColumnName] = time.Now().UnixNano()
			}

			samples = append(samples, measurement)
		}

		result = append(result, metrics.MeasurementEnvelope{
			MetricName: familyName,
			SourceKind: string(sc.Kind),
			Data:       samples,
		})
	}
	return result, nil
}

// promMetricValue extracts the primary float64 sample value from a metric
// based on its family type.
func promMetricValue(m *dto.Metric, mtype dto.MetricType) float64 {
	switch mtype {
	case dto.MetricType_GAUGE:
		return m.GetGauge().GetValue()
	case dto.MetricType_COUNTER:
		return m.GetCounter().GetValue()
	case dto.MetricType_HISTOGRAM:
		return float64(m.GetHistogram().GetSampleCount())
	case dto.MetricType_SUMMARY:
		return m.GetSummary().GetSampleSum()
	default:
		return m.GetUntyped().GetValue()
	}
}
