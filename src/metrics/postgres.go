package metrics

import (
	"context"

	"github.com/cybertec-postgresql/pgwatch3/db"
	"github.com/cybertec-postgresql/pgwatch3/log"
)

func NewPostgresMetricReaderWriter(ctx context.Context, conn db.PgxPoolIface) (ReaderWriter, error) {
	if exists, err := db.DoesSchemaExist(ctx, conn, "pgwatch3"); err == nil {
		if !exists {
			tx, err := conn.Begin(ctx)
			if err != nil {
				return nil, err
			}
			defer func() { _ = tx.Rollback(ctx) }()
			if _, err := tx.Exec(ctx, db.SQLConfigSchema); err != nil {
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

type dbMetricReaderWriter struct {
	ctx      context.Context
	configDb db.PgxIface
}

// writeMetricsToPostgres writes the metrics and presets definitions to the
// pgwatch3.metric and pgwatch3.preset tables in the ConfigDB.
func writeMetricsToPostgres(ctx context.Context, conn db.PgxIface, metricDefs *Metrics) error {
	logger := log.GetLogger(ctx)
	logger.Info("writing metrics definitions to configuration database...")
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for metricName, metric := range metricDefs.MetricDefs {
		_, err = tx.Exec(ctx, `INSERT INTO pgwatch3.metric (name, sqls, init_sql, description, node_status, gauges, is_instance_level, storage_name)
		values ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT (name) 
		DO UPDATE SET sqls = $2, init_sql = $3, description = $4, node_status = $5, 
		gauges = $6, is_instance_level = $7, storage_name = $8`,
			metricName, metric.SQLs, metric.InitSQL, metric.Description, metric.NodeStatus, metric.Gauges, metric.IsInstanceLevel, metric.StorageName)
		if err != nil {
			return err
		}
	}
	for presetName, preset := range metricDefs.PresetDefs {
		_, err = tx.Exec(ctx, `INSERT INTO pgwatch3.preset (name, description, metrics) 
		VALUES ($1, $2, $3) ON CONFLICT (name) DO UPDATE SET description = $2, metrics = $3;`,
			presetName, preset.Description, preset.Metrics)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ReadMetricsFromPostgres reads the metrics and presets definitions from the pgwatch3.metric and pgwatch3.preset tables from the ConfigDB.
func (dmrw *dbMetricReaderWriter) GetMetrics() (metricDefMapNew *Metrics, err error) {
	ctx := dmrw.ctx
	conn := dmrw.configDb
	logger := log.GetLogger(ctx)
	logger.Info("reading metrics definitions from configuration database...")
	metricDefMapNew = &Metrics{MetricDefs{}, PresetDefs{}}
	rows, err := conn.Query(ctx, `SELECT name, sqls, init_sql, description, node_status, gauges, is_instance_level, storage_name FROM pgwatch3.metric`)
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
	rows, err = conn.Query(ctx, `SELECT name, description, metrics FROM pgwatch3.preset`)
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
	_, err := dmrw.configDb.Exec(dmrw.ctx, `DELETE FROM pgwatch3.metric WHERE name = $1`, metricName)
	return err
}

func (dmrw *dbMetricReaderWriter) UpdateMetric(metricName string, metric Metric) error {
	_, err := dmrw.configDb.Exec(dmrw.ctx, `UPDATE pgwatch3.metric SET 
	sqls = $2, init_sql = $3, description = $4, node_status = $5, gauges = $6, is_instance_level = $7, storage_name = $8
	WHERE name = $1`,
		metricName, metric.SQLs, metric.InitSQL, metric.Description,
		metric.NodeStatus, metric.Gauges, metric.IsInstanceLevel, metric.StorageName)
	return err
}

func (dmrw *dbMetricReaderWriter) DeletePreset(presetName string) error {
	_, err := dmrw.configDb.Exec(dmrw.ctx, `DELETE FROM pgwatch3.preset WHERE name = $1`, presetName)
	return err
}

func (dmrw *dbMetricReaderWriter) UpdatePreset(presetName string, preset Preset) error {
	_, err := dmrw.configDb.Exec(dmrw.ctx, `UPDATE pgwatch3.preset SET description = $2, metrics = $3 WHERE name = $1`,
		presetName, preset.Description, preset.Metrics)
	return err
}
