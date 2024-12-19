package sinks

import "time"

// CmdOpts specifies the storage configuration to store metrics measurements
type CmdOpts struct {
	Sinks                 []string      `long:"sink" mapstructure:"sink" description:"URI where metrics will be stored, can be used multiple times" env:"PW_SINK"`
	BatchingDelay         time.Duration `long:"batching-delay" mapstructure:"batching-delay" description:"Max milliseconds to wait for a batched metrics flush. [Default: 250ms]" default:"250ms" env:"PW_BATCHING_MAX_DELAY"`
	Retention             int           `long:"retention" mapstructure:"retention" description:"If set, metrics older than that will be deleted" default:"14" env:"PW_RETENTION"`
	RealDbnameField       string        `long:"real-dbname-field" mapstructure:"real-dbname-field" description:"Tag key for real database name" env:"PW_REAL_DBNAME_FIELD" default:"real_dbname"`
	SystemIdentifierField string        `long:"system-identifier-field" mapstructure:"system-identifier-field" description:"Tag key for system identifier value" env:"PW_SYSTEM_IDENTIFIER_FIELD" default:"sys_id"`
}
