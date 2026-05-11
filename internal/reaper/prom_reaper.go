package reaper

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
)

const defaultScrapeInterval = 60 * time.Second

var _ Reaper = (*PromReaper)(nil)

// PromReaper drives metric scraping for a single Prometheus source.
// It runs a GCD-based tick loop and applies per-family emit-interval gating.
type PromReaper struct {
	reaper      *reaper
	md          *sources.PromConn
	lastEmitted map[string]time.Time
}

// NewPromSourceReaper creates a PromReaper for the given Prometheus source.
func NewPromSourceReaper(r *reaper, md *sources.PromConn) *PromReaper {
	return &PromReaper{
		reaper:      r,
		md:          md,
		lastEmitted: make(map[string]time.Time),
	}
}

// calcScrapeInterval returns the GCD of all configured metric intervals.
// Defaults to defaultScrapeInterval when no metrics are configured (scrape-all mode).
// Individual intervals below minTickInterval are floored to minTickInterval.
func (pr *PromReaper) calcScrapeInterval() time.Duration {
	pr.md.RLock()
	m := pr.md.Metrics
	pr.md.RUnlock()

	if len(m) == 0 {
		return defaultScrapeInterval
	}
	intervals := make([]int, 0, len(m))
	for _, v := range m {
		intervals = append(intervals, max(v, minTickInterval))
	}
	return time.Duration(max(GCDSlice(intervals), minTickInterval)) * time.Second
}

// Reap is the main loop for a Prometheus source. It scrapes all metric families
// on every GCD tick and emits envelopes that have passed their per-family interval.
func (pr *PromReaper) Reap(ctx context.Context) {
	l := log.GetLogger(ctx).WithField("source", pr.md.Name)
	ctx = log.WithLogger(ctx, l)

	pr.md.RLock()
	scrapeAll := len(pr.md.Metrics) == 0
	pr.md.RUnlock()

	if scrapeAll {
		l.Warning("no metrics configured for prometheus source, using scrape-all mode with 60 s interval")
	}

	for {
		envelopes, err := pr.ScrapeAll(ctx)
		if err != nil {
			l.WithError(err).Warning("prometheus scrape failed")
		} else {
			now := time.Now()
			for _, env := range envelopes {
				if !scrapeAll {
					emitInterval := pr.md.GetMetricInterval(env.MetricName)
					if emitInterval > 0 {
						if last := pr.lastEmitted[env.MetricName]; !last.IsZero() && now.Sub(last) < emitInterval {
							continue
						}
					}
				}
				env.DBName = pr.md.Name
				env.CustomTags = pr.md.CustomTags
				env.SourceKind = string(sources.SourcePrometheus)
				pr.reaper.measurementCh <- env
				pr.lastEmitted[env.MetricName] = now
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(pr.calcScrapeInterval()):
		}
	}
}

// ScrapeAll fetches Prometheus exposition metrics from pr.md and returns one
// MeasurementEnvelope per metric family. Each sample becomes one Measurement
// with tag_<label> columns (skipping __name__), a value column named after the
// family, and epoch_ns set from the sample timestamp (ms→ns) or time.Now().
func (pr *PromReaper) ScrapeAll(ctx context.Context) ([]metrics.MeasurementEnvelope, error) {
	resp, err := pr.md.Scrape(ctx)
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
			SourceKind: string(pr.md.Kind),
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
