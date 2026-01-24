package cmdopts

// ErrUpgradeNotSupported is returned when a config backend
// does not support schema upgrades (e.g. YAML files).
type ErrUpgradeNotSupported struct {
	Target string // sources.yaml / metrics.yaml / sinks
}

func (e *ErrUpgradeNotSupported) Error() string {
	return "configuration storage does not support upgrade"
}
