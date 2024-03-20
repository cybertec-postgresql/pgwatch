package metrics

import (
	"context"

	"github.com/cybertec-postgresql/pgwatch3/config"
	"github.com/cybertec-postgresql/pgwatch3/db"
	"github.com/cybertec-postgresql/pgwatch3/log"
)

func NewPostgresMetricReader(ctx context.Context, conn db.PgxPoolIface, opts *config.Options) (Reader, error) {
	if exists, err := db.DoesSchemaExist(ctx, conn, "pgwatch3"); err == nil {
		if !exists {
			tx, err := conn.Begin(ctx)
			if err != nil {
				return nil, err
			}
			defer tx.Rollback(ctx)
			if _, err := tx.Exec(ctx, db.SqlConfigSchema); err != nil {
				return nil, err
			}
			if err := WriteMetricsToPostgres(ctx, tx, GetDefaultMetrics()); err != nil {
				return nil, err
			}
			if err := tx.Commit(ctx); err != nil {
				return nil, err
			}
		}
	} else {
		return nil, err
	}

	return &dbMetricReader{
		ctx:      ctx,
		configDb: conn,
		opts:     opts,
	}, conn.Ping(ctx)
}

type dbMetricReader struct {
	ctx      context.Context
	configDb db.Querier
	opts     *config.Options
}

// WriteMetricsToPostgres writes the metrics and presets definitions to the
// pgwatch3.metric and pgwatch3.preset tables in the ConfigDB.
func WriteMetricsToPostgres(ctx context.Context, conn db.PgxIface, metricDefs *Metrics) error {
	logger := log.GetLogger(ctx)
	logger.Info("writing metrics definitions to configuration database...")
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for metricName, metric := range metricDefs.MetricDefs {
		_, err = tx.Exec(ctx, `INSERT INTO pgwatch3.metric (name, sqls, description, enabled, node_status, gauges, is_instance_level, storage_name)
		values ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT (name, node_status) 
		DO UPDATE SET sqls = $2, description = $3, enabled = $4, gauges = $6, is_instance_level = $7, storage_name = $8`,
			metricName, metric.SQLs, metric.Description, metric.Enabled, metric.NodeStatus, metric.Gauges, metric.IsInstanceLevel, metric.StorageName)
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
func (r *dbMetricReader) GetMetrics() (metricDefMapNew *Metrics, err error) {
	ctx := r.ctx
	conn := r.configDb
	logger := log.GetLogger(ctx)
	logger.Info("reading metrics definitions from configuration database...")
	metricDefMapNew = &Metrics{MetricDefs{}, PresetDefs{}}
	rows, err := conn.Query(ctx, `SELECT name, sqls, description, enabled, node_status, gauges, is_instance_level, storage_name FROM pgwatch3.metric`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		metric := Metric{}
		var name string
		err = rows.Scan(&name, &metric.SQLs, &metric.Description, &metric.Enabled, &metric.NodeStatus, &metric.Gauges, &metric.IsInstanceLevel, &metric.StorageName)
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
