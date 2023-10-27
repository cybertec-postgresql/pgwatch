package db

import (
	"context"
	"regexp"
	"strconv"

	"github.com/cybertec-postgresql/pgwatch3/log"
	"github.com/cybertec-postgresql/pgwatch3/metrics"
	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v2"
)

type MetricSchemaType int

const (
	MetricSchemaPostgres MetricSchemaType = iota
	MetricSchemaTimescale
)

func GetMetricSchemaType(ctx context.Context, conn PgxIface) (metricSchema MetricSchemaType, err error) {
	var isTs bool
	metricSchema = MetricSchemaPostgres
	sqlSchemaType := `SELECT schema_type = 'timescale' FROM admin.storage_schema_type`
	if err = conn.QueryRow(ctx, sqlSchemaType).Scan(&isTs); err == nil && isTs {
		metricSchema = MetricSchemaTimescale
	}
	return
}

func versionToInt(v string) uint {
	if len(v) == 0 {
		return 0
	}
	var major int
	var minor int

	matches := regexp.MustCompile(`(\d+).?(\d*)`).FindStringSubmatch(v)
	if len(matches) == 0 {
		return 0
	}
	if len(matches) > 1 {
		major, _ = strconv.Atoi(matches[1])
		if len(matches) > 2 {
			minor, _ = strconv.Atoi(matches[2])
		}
	}
	return uint(major*100 + minor)
}

func ReadMetricsFromPostgres(ctx context.Context, conn PgxIface) (
	metricDefMapNew map[string]map[uint]metrics.MetricProperties,
	metricNameRemapsNew map[string]string,
	err error) {

	sql := `select /* pgwatch3_generated */ m_name, m_pg_version_from::text, m_sql, m_master_only, m_standby_only,
			  coalesce(m_column_attrs::text, '') as m_column_attrs, coalesce(m_column_attrs::text, '') as m_column_attrs,
			  coalesce(ma_metric_attrs::text, '') as ma_metric_attrs, m_sql_su
			from
              pgwatch3.metric
              left join
              pgwatch3.metric_attribute on (ma_metric_name = m_name)
			where
              m_is_active
		    order by
		      1, 2`
	logger := log.GetLogger(ctx)
	logger.Info("updating metrics definitons from ConfigDB...")

	var (
		data []map[string]any
		rows pgx.Rows
	)
	if rows, err = conn.Query(ctx, sql); err == nil {
		data, err = pgx.CollectRows(rows, pgx.RowToMap)
	}
	if err != nil {
		return
	}
	if len(data) == 0 {
		logger.Warning("no active metric definitions found from config DB")
		return
	}
	metricDefMapNew = make(map[string]map[uint]metrics.MetricProperties)
	metricNameRemapsNew = make(map[string]string)
	logger.WithField("metrics", len(data)).Debug("Active metrics found in config database (pgwatch3.metric)")
	for _, row := range data {
		_, ok := metricDefMapNew[row["m_name"].(string)]
		if !ok {
			metricDefMapNew[row["m_name"].(string)] = make(map[uint]metrics.MetricProperties)
		}
		d := versionToInt(row["m_pg_version_from"].(string))
		ca := metrics.MetricPrometheusAttrs{}
		if row["m_column_attrs"].(string) != "" {
			_ = yaml.Unmarshal([]byte(row["m_column_attrs"].(string)), &ca)
		}
		ma := metrics.MetricAttrs{}
		if row["ma_metric_attrs"].(string) != "" {
			_ = yaml.Unmarshal([]byte(row["ma_metric_attrs"].(string)), &ma)
			if ma.MetricStorageName != "" {
				metricNameRemapsNew[row["m_name"].(string)] = ma.MetricStorageName
			}
		}
		metricDefMapNew[row["m_name"].(string)][d] = metrics.MetricProperties{
			SQL:                  row["m_sql"].(string),
			SQLSU:                row["m_sql_su"].(string),
			MasterOnly:           row["m_master_only"].(bool),
			StandbyOnly:          row["m_standby_only"].(bool),
			PrometheusAttrs:      ca,
			MetricAttrs:          ma,
			CallsHelperFunctions: metrics.DoesMetricDefinitionCallHelperFunctions(row["m_sql"].(string)),
		}
	}
	return
}
