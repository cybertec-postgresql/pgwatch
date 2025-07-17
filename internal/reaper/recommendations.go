package reaper

const (
	specialMetricChangeEvents         = "change_events"
	specialMetricServerLogEventCounts = "server_log_event_counts"
	specialMetricInstanceUp           = "instance_up"
)

var specialMetrics = map[string]bool{specialMetricChangeEvents: true, specialMetricServerLogEventCounts: true}
