package sinks

import "time"

// CmdOpts specifies the storage configuration to store metrics measurements
type CmdOpts struct {
	Sinks                 []string      `long:"sink" mapstructure:"sink" description:"URI where metrics will be stored, can be used multiple times" env:"PW_SINK"`
	BatchingDelay         time.Duration `long:"batching-delay" mapstructure:"batching-delay" description:"Sink-specific batching flush delay; may be ignored by some sinks" default:"950ms" env:"PW_BATCHING_DELAY"`
	PartitionInterval     string        `long:"partition-interval" mapstructure:"partition-interval" description:"Time range for PostgreSQL sink time partitions. Must be a valid PostgreSQL interval." default:"1 week" env:"PW_PARTITION_INTERVAL"`
	Retention             string        `long:"retention" mapstructure:"retention" description:"Delete metrics older than this. set to zero to disable. Must be a valid PostgreSQL interval." default:"14 days" env:"PW_RETENTION"`
	MaintenanceInterval   string        `long:"maintenance-interval" mapstructure:"maintenance-interval" description:"Run maintenance tasks (e.g., removing old metrics) with this interval; set to zero to disable. Must be a valid PostgreSQL interval." default:"12 hours" env:"PW_MAINTENANCE_INTERVAL"`
	RealDbnameField       string        `long:"real-dbname-field" mapstructure:"real-dbname-field" description:"Tag key for real database name" env:"PW_REAL_DBNAME_FIELD" default:"real_dbname"`
	SystemIdentifierField string        `long:"system-identifier-field" mapstructure:"system-identifier-field" description:"Tag key for system identifier value" env:"PW_SYSTEM_IDENTIFIER_FIELD" default:"sys_id"`
}
