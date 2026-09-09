package metrics

import (
	"time"
)

// CmdOpts specifies metric command-line options
type CmdOpts struct {
	Metrics                      string `short:"m" long:"metrics" mapstructure:"metrics" description:"Postgres URI or path to YAML file with metrics definitions" env:"PW_METRICS"`
	DirectOSStats                bool   `hidden:"true" long:"direct-os-stats" mapstructure:"direct-os-stats" description:"Extract OS related psutil statistics not via PL/Python wrappers but directly on host" env:"PW_DIRECT_OS_STATS"`
	InstanceLevelCacheMaxSeconds int64  `long:"instance-level-cache-max-seconds" mapstructure:"instance-level-cache-max-seconds" description:"Max allowed staleness for instance level metric data shared between DBs of an instance. Set to 0 to disable" env:"PW_INSTANCE_LEVEL_CACHE_MAX_SECONDS" default:"30"`
	EmergencyPauseTriggerfile    string `long:"emergency-pause-triggerfile" mapstructure:"emergency-pause-triggerfile" description:"When the file exists no metrics will be temporarily fetched" env:"PW_EMERGENCY_PAUSE_TRIGGERFILE" default:"/tmp/pgwatch-emergency-pause"`
}

func (c CmdOpts) CacheAge() time.Duration {
	if c.InstanceLevelCacheMaxSeconds < 0 {
		c.InstanceLevelCacheMaxSeconds = 0
	}
	return time.Duration(c.InstanceLevelCacheMaxSeconds) * time.Second
}
