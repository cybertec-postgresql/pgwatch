package metrics

import (
	"context"
	_ "embed"

	"github.com/cybertec-postgresql/pgwatch/db"
)

func NewPostgresMetricReaderWriter(ctx context.Context, connstr string) (ReaderWriter, error) {
	conn, err := db.New(ctx, connstr)
	if err != nil {
		return nil, err
	}
	return NewPostgresMetricReaderWriterConn(ctx, conn)
}

func NewPostgresMetricReaderWriterConn(ctx context.Context, conn db.PgxPoolIface) (ReaderWriter, error) {
	if exists, err := db.DoesSchemaExist(ctx, conn, "pgwatch"); err == nil {
		if !exists {
			tx, err := conn.Begin(ctx)
			if err != nil {
				return nil, err
			}
			defer func() { _ = tx.Rollback(ctx) }()
			if _, err := tx.Exec(ctx, sqlConfigSchema); err != nil {
				return nil, err
			}
			if err := writeMetricsToPostgres(ctx, tx, GetDefaultMetrics()); err != nil {
				return nil, err
			}
			if err := tx.Commit(ctx); err != nil {
				return nil, err
			}
		}
	} else {
		return nil, err
	}

	return &dbMetricReaderWriter{
		ctx:      ctx,
		configDb: conn,
	}, conn.Ping(ctx)
}

//go:embed postgres_schema.sql
var sqlConfigSchema string

type dbMetricReaderWriter struct {
	ctx      context.Context
	configDb db.PgxIface
}

// make sure *dbMetricReaderWriter implements the Migrator interface
var _ Migrator = (*dbMetricReaderWriter)(nil)

// writeMetricsToPostgres writes the metrics and presets definitions to the
// pgwatch.metric and pgwatch.preset tables in the ConfigDB.
func writeMetricsToPostgres(ctx context.Context, conn db.PgxIface, metricDefs *Metrics) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for metricName, metric := range metricDefs.MetricDefs {
		_, err = tx.Exec(ctx, `INSERT INTO pgwatch.metric (name, sqls, init_sql, description, node_status, gauges, is_instance_level, storage_name)
		values ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT (name) 
		DO UPDATE SET sqls = $2, init_sql = $3, description = $4, node_status = $5, 
		gauges = $6, is_instance_level = $7, storage_name = $8`,
			metricName, metric.SQLs, metric.InitSQL, metric.Description, metric.NodeStatus, metric.Gauges, metric.IsInstanceLevel, metric.StorageName)
		if err != nil {
			return err
		}
	}
	for presetName, preset := range metricDefs.PresetDefs {
		_, err = tx.Exec(ctx, `INSERT INTO pgwatch.preset (name, description, metrics) 
		VALUES ($1, $2, $3) ON CONFLICT (name) DO UPDATE SET description = $2, metrics = $3;`,
			presetName, preset.Description, preset.Metrics)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ReadMetricsFromPostgres reads the metrics and presets definitions from the pgwatch.metric and pgwatch.preset tables from the ConfigDB.
func (dmrw *dbMetricReaderWriter) GetMetrics() (metricDefMapNew *Metrics, err error) {
	ctx := dmrw.ctx
	conn := dmrw.configDb
	metricDefMapNew = &Metrics{MetricDefs{}, PresetDefs{}}
	rows, err := conn.Query(ctx, `SELECT name, sqls, init_sql, description, node_status, gauges, is_instance_level, storage_name FROM pgwatch.metric`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		metric := Metric{}
		var name string
		err = rows.Scan(&name, &metric.SQLs, &metric.InitSQL, &metric.Description, &metric.NodeStatus, &metric.Gauges, &metric.IsInstanceLevel, &metric.StorageName)
		if err != nil {
			return nil, err
		}
		metricDefMapNew.MetricDefs[name] = metric
	}
	rows, err = conn.Query(ctx, `SELECT name, description, metrics FROM pgwatch.preset`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		preset := Preset{}
		var name string
		err = rows.Scan(&name, &preset.Description, &preset.Metrics)
		if err != nil {
			return nil, err
		}
		metricDefMapNew.PresetDefs[name] = preset
	}
	return metricDefMapNew, nil
}

func (dmrw *dbMetricReaderWriter) WriteMetrics(metricDefs *Metrics) error {
	return writeMetricsToPostgres(dmrw.ctx, dmrw.configDb, metricDefs)
}

func (dmrw *dbMetricReaderWriter) DeleteMetric(metricName string) error {
	_, err := dmrw.configDb.Exec(dmrw.ctx, `DELETE FROM pgwatch.metric WHERE name = $1`, metricName)
	return err
}

func (dmrw *dbMetricReaderWriter) UpdateMetric(metricName string, metric Metric) error {
	_, err := dmrw.configDb.Exec(dmrw.ctx, `UPDATE pgwatch.metric SET 
	sqls = $2, init_sql = $3, description = $4, node_status = $5, gauges = $6, is_instance_level = $7, storage_name = $8
	WHERE name = $1`,
		metricName, db.MarshallParam(metric.SQLs), metric.InitSQL, metric.Description,
		metric.NodeStatus, db.MarshallParam(metric.Gauges), metric.IsInstanceLevel, metric.StorageName)
	return err
}

func (dmrw *dbMetricReaderWriter) DeletePreset(presetName string) error {
	_, err := dmrw.configDb.Exec(dmrw.ctx, `DELETE FROM pgwatch.preset WHERE name = $1`, presetName)
	return err
}

func (dmrw *dbMetricReaderWriter) UpdatePreset(presetName string, preset Preset) error {
	sql := `INSERT INTO pgwatch.preset(name, description, metrics) VALUES ($1, $2, $3)
	ON CONFLICT (name) DO UPDATE SET description = $2, metrics = $3`
	_, err := dmrw.configDb.Exec(dmrw.ctx, sql, presetName, preset.Description, db.MarshallParam(preset.Metrics))
	return err
}
