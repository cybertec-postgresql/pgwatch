package db

import (
	//use blank embed import
	_ "embed"
)

//go:embed sql/config/config_schema.sql
var SQLConfigSchema string

//go:embed sql/metric/admin_schema.sql
var sqlMetricAdminSchema string

//go:embed sql/metric/admin_functions.sql
var sqlMetricAdminFunctions string

//go:embed sql/metric/ensure_partition_postgres.sql
var sqlMetricEnsurePartitionPostgres string

//go:embed sql/metric/ensure_partition_timescale.sql
var sqlMetricEnsurePartitionTimescale string

//go:embed sql/metric/change_chunk_interval.sql
var sqlMetricChangeChunkIntervalTimescale string

//go:embed sql/metric/change_compression_interval.sql
var sqlMetricChangeCompressionIntervalTimescale string
