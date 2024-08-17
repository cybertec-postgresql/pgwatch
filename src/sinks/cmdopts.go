package sinks

import "time"

// SinkCmdOpts specifies the storage configuration to store metrics measurements
type SinkCmdOpts struct {
	Sinks                 []string      `long:"sink" mapstructure:"sink" description:"URI where metrics will be stored" env:"PW3_SINK"`
	BatchingDelay         time.Duration `long:"batching-delay" mapstructure:"batching-delay" description:"Max milliseconds to wait for a batched metrics flush. [Default: 250ms]" default:"250ms" env:"PW3_BATCHING_MAX_DELAY"`
	Retention             int           `long:"retention" mapstructure:"retention" description:"If set, metrics older than that will be deleted" default:"14" env:"PW3_RETENTION"`
	RealDbnameField       string        `long:"real-dbname-field" mapstructure:"real-dbname-field" description:"Tag key for real DB name if --add-real-dbname enabled" env:"PW3_REAL_DBNAME_FIELD" default:"real_dbname"`
	SystemIdentifierField string        `long:"system-identifier-field" mapstructure:"system-identifier-field" description:"Tag key for system identifier value if --add-system-identifier" env:"PW3_SYSTEM_IDENTIFIER_FIELD" default:"sys_id"`
}