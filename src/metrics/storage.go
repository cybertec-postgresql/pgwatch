package metrics

type MetricWriter interface {
	WriteMetric(m MetricData) (n int, err error)
}
