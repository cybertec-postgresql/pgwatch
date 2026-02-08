package reaper

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sinks"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
	"github.com/jackc/pgx/v5"
)

func QueryMeasurements(ctx context.Context, md *sources.SourceConn, sql string, args ...any) (metrics.Measurements, error) {
	if strings.TrimSpace(sql) == "" {
		return nil, errors.New("empty SQL")
	}

	// For non-postgres connections (e.g. pgbouncer, pgpool), use simple protocol
	if !md.IsPostgresSource() {
		args = append([]any{pgx.QueryExecModeSimpleProtocol}, args...)
	}
	// lock_timeout is set at connection level via RuntimeParams, no need for transaction wrapper
	rows, err := md.Conn.Query(ctx, sql, args...)
	if err == nil {
		return pgx.CollectRows(rows, metrics.RowToMeasurement)
	}
	return nil, err
}

func (r *Reaper) DetectSprocChanges(ctx context.Context, md *sources.SourceConn) (changeCounts ChangeDetectionResults) {
	detectedChanges := make(metrics.Measurements, 0)
	var firstRun bool
	l := log.GetLogger(ctx)
	changeCounts.Target = "functions"
	l.Debug("checking for sproc changes...")
	if _, ok := md.ChangeState["sproc_hashes"]; !ok {
		firstRun = true
		md.ChangeState["sproc_hashes"] = make(map[string]string)
	}

	mvp, ok := metricDefs.GetMetricDef("sproc_hashes")
	if !ok {
		l.Error("could not get sproc_hashes sql")
		return
	}

	data, err := QueryMeasurements(ctx, md, mvp.GetSQL(int(md.Version)))
	if err != nil {
		l.Error(err)
		return
	}

	for _, dr := range data {
		objIdent := dr["tag_sproc"].(string) + dbMetricJoinStr + dr["tag_oid"].(string)
		prevHash, ok := md.ChangeState["sproc_hashes"][objIdent]
		ll := l.WithField("sproc", dr["tag_sproc"]).WithField("oid", dr["tag_oid"])
		if ok { // we have existing state
			if prevHash != dr["md5"].(string) {
				ll.Debug("change detected")
				dr["event"] = "alter"
				detectedChanges = append(detectedChanges, dr)
				md.ChangeState["sproc_hashes"][objIdent] = dr["md5"].(string)
				changeCounts.Altered++
			}
		} else { // check for new / delete
			if !firstRun {
				ll.Debug("new sproc detected")
				dr["event"] = "create"
				detectedChanges = append(detectedChanges, dr)
				changeCounts.Created++
			}
			md.ChangeState["sproc_hashes"][objIdent] = dr["md5"].(string)
		}
	}
	// detect deletes
	if !firstRun && len(md.ChangeState["sproc_hashes"]) != len(data) {
		// turn resultset to map => [oid]=true for faster checks
		currentOidMap := make(map[string]bool)
		for _, dr := range data {
			currentOidMap[dr["tag_sproc"].(string)+dbMetricJoinStr+dr["tag_oid"].(string)] = true
		}
		for sprocIdent := range md.ChangeState["sproc_hashes"] {
			_, ok := currentOidMap[sprocIdent]
			if !ok {
				splits := strings.Split(sprocIdent, dbMetricJoinStr)
				l.WithField("sproc", splits[0]).WithField("oid", splits[1]).Debug("deleted sproc detected")
				m := metrics.NewMeasurement(data.GetEpoch())
				m["event"] = "drop"
				m["tag_sproc"] = splits[0]
				m["tag_oid"] = splits[1]
				detectedChanges = append(detectedChanges, m)
				delete(md.ChangeState["sproc_hashes"], sprocIdent)
				changeCounts.Dropped++
			}
		}
	}
	l.Debugf("sproc changes detected: %d", len(detectedChanges))
	if len(detectedChanges) > 0 {
		r.measurementCh <- metrics.MeasurementEnvelope{
			DBName:     md.Name,
			MetricName: "sproc_changes",
			Data:       detectedChanges,
			CustomTags: md.CustomTags,
		}
	}

	return changeCounts
}

func (r *Reaper) DetectTableChanges(ctx context.Context, md *sources.SourceConn) ChangeDetectionResults {
	detectedChanges := make(metrics.Measurements, 0)
	var firstRun bool
	var changeCounts ChangeDetectionResults
	l := log.GetLogger(ctx)
	changeCounts.Target = "tables"
	l.Debug("checking for table changes...")
	if _, ok := md.ChangeState["table_hashes"]; !ok {
		firstRun = true
		md.ChangeState["table_hashes"] = make(map[string]string)
	}

	mvp, ok := metricDefs.GetMetricDef("table_hashes")
	if !ok {
		l.Error("could not get table_hashes sql")
		return changeCounts
	}

	data, err := QueryMeasurements(ctx, md, mvp.GetSQL(int(md.Version)))
	if err != nil {
		l.Error(err)
		return changeCounts
	}

	for _, dr := range data {
		objIdent := dr["tag_table"].(string)
		prevHash, ok := md.ChangeState["table_hashes"][objIdent]
		ll := l.WithField("table", dr["tag_table"])
		if ok { // we have existing state
			if prevHash != dr["md5"].(string) {
				ll.Debug("change detected")
				dr["event"] = "alter"
				detectedChanges = append(detectedChanges, dr)
				md.ChangeState["table_hashes"][objIdent] = dr["md5"].(string)
				changeCounts.Altered++
			}
		} else { // check for new / delete
			if !firstRun {
				ll.Debug("new table detected")
				dr["event"] = "create"
				detectedChanges = append(detectedChanges, dr)
				changeCounts.Created++
			}
			md.ChangeState["table_hashes"][objIdent] = dr["md5"].(string)
		}
	}
	// detect deletes
	if !firstRun && len(md.ChangeState["table_hashes"]) != len(data) {
		deletedTables := make([]string, 0)
		// turn resultset to map => [table]=true for faster checks
		currentTableMap := make(map[string]bool)
		for _, dr := range data {
			currentTableMap[dr["tag_table"].(string)] = true
		}
		for table := range md.ChangeState["table_hashes"] {
			_, ok := currentTableMap[table]
			if !ok {
				l.WithField("table", table).Debug("deleted table detected")
				influxEntry := metrics.NewMeasurement(data.GetEpoch())
				influxEntry["event"] = "drop"
				influxEntry["tag_table"] = table
				detectedChanges = append(detectedChanges, influxEntry)
				deletedTables = append(deletedTables, table)
				changeCounts.Dropped++
			}
		}
		for _, deletedTable := range deletedTables {
			delete(md.ChangeState["table_hashes"], deletedTable)
		}
	}

	l.Debugf("table changes detected: %d", len(detectedChanges))
	if len(detectedChanges) > 0 {
		r.measurementCh <- metrics.MeasurementEnvelope{
			DBName:     md.Name,
			MetricName: "table_changes",
			Data:       detectedChanges,
			CustomTags: md.CustomTags,
		}
	}

	return changeCounts
}

func (r *Reaper) DetectIndexChanges(ctx context.Context, md *sources.SourceConn) ChangeDetectionResults {
	detectedChanges := make(metrics.Measurements, 0)
	var firstRun bool
	var changeCounts ChangeDetectionResults
	l := log.GetLogger(ctx)
	changeCounts.Target = "indexes"
	l.Debug("checking for index changes...")
	if _, ok := md.ChangeState["index_hashes"]; !ok {
		firstRun = true
		md.ChangeState["index_hashes"] = make(map[string]string)
	}

	mvp, ok := metricDefs.GetMetricDef("index_hashes")
	if !ok {
		l.Error("could not get index_hashes sql")
		return changeCounts
	}

	data, err := QueryMeasurements(ctx, md, mvp.GetSQL(int(md.Version)))
	if err != nil {
		l.Error(err)
		return changeCounts
	}

	for _, dr := range data {
		objIdent := dr["tag_index"].(string)
		prevHash, ok := md.ChangeState["index_hashes"][objIdent]
		ll := l.WithField("index", dr["tag_index"]).WithField("table", dr["table"])
		if ok { // we have existing state
			if prevHash != (dr["md5"].(string) + dr["is_valid"].(string)) {
				ll.Debug("change detected")
				dr["event"] = "alter"
				detectedChanges = append(detectedChanges, dr)
				md.ChangeState["index_hashes"][objIdent] = dr["md5"].(string) + dr["is_valid"].(string)
				changeCounts.Altered++
			}
		} else { // check for new / delete
			if !firstRun {
				ll.Debug("new index detected")
				dr["event"] = "create"
				detectedChanges = append(detectedChanges, dr)
				changeCounts.Created++
			}
			md.ChangeState["index_hashes"][objIdent] = dr["md5"].(string) + dr["is_valid"].(string)
		}
	}
	// detect deletes
	if !firstRun && len(md.ChangeState["index_hashes"]) != len(data) {
		deletedIndexes := make([]string, 0)
		// turn resultset to map => [table]=true for faster checks
		currentIndexMap := make(map[string]bool)
		for _, dr := range data {
			currentIndexMap[dr["tag_index"].(string)] = true
		}
		for indexName := range md.ChangeState["index_hashes"] {
			_, ok := currentIndexMap[indexName]
			if !ok {
				l.WithField("index", indexName).Debug("deleted index detected")
				influxEntry := metrics.NewMeasurement(data.GetEpoch())
				influxEntry["event"] = "drop"
				influxEntry["tag_index"] = indexName
				detectedChanges = append(detectedChanges, influxEntry)
				deletedIndexes = append(deletedIndexes, indexName)
				changeCounts.Dropped++
			}
		}
		for _, deletedIndex := range deletedIndexes {
			delete(md.ChangeState["index_hashes"], deletedIndex)
		}
	}
	l.Debugf("index changes detected: %d", len(detectedChanges))
	if len(detectedChanges) > 0 {
		r.measurementCh <- metrics.MeasurementEnvelope{
			DBName:     md.Name,
			MetricName: "index_changes",
			Data:       detectedChanges,
			CustomTags: md.CustomTags,
		}
	}

	return changeCounts
}

func (r *Reaper) DetectPrivilegeChanges(ctx context.Context, md *sources.SourceConn) ChangeDetectionResults {
	detectedChanges := make(metrics.Measurements, 0)
	var firstRun bool
	var changeCounts ChangeDetectionResults
	l := log.GetLogger(ctx)
	changeCounts.Target = "privileges"
	l.Debug("checking object privilege changes...")
	if _, ok := md.ChangeState["object_privileges"]; !ok {
		firstRun = true
		md.ChangeState["object_privileges"] = make(map[string]string)
	}

	mvp, ok := metricDefs.GetMetricDef("privilege_changes")
	if !ok || mvp.GetSQL(int(md.Version)) == "" {
		l.Warning("could not get SQL for 'privilege_changes'. cannot detect privilege changes")
		return changeCounts
	}

	// returns rows of: object_type, tag_role, tag_object, privilege_type
	data, err := QueryMeasurements(ctx, md, mvp.GetSQL(int(md.Version)))
	if err != nil {
		l.Error(err)
		return changeCounts
	}

	currentState := make(map[string]bool)
	for _, dr := range data {
		objIdent := fmt.Sprintf("%s#:#%s#:#%s#:#%s", dr["object_type"], dr["tag_role"], dr["tag_object"], dr["privilege_type"])
		ll := l.WithField("role", dr["tag_role"]).
			WithField("object_type", dr["object_type"]).
			WithField("object", dr["tag_object"]).
			WithField("privilege_type", dr["privilege_type"])
		if firstRun {
			md.ChangeState["object_privileges"][objIdent] = ""
		} else {
			_, ok := md.ChangeState["object_privileges"][objIdent]
			if !ok {
				ll.Debug("new object privileges detected")
				dr["event"] = "GRANT"
				detectedChanges = append(detectedChanges, dr)
				changeCounts.Created++
				md.ChangeState["object_privileges"][objIdent] = ""
			}
			currentState[objIdent] = true
		}
	}
	// check revokes - exists in old state only
	if !firstRun && len(currentState) > 0 {
		for objPrevRun := range md.ChangeState["object_privileges"] {
			if _, ok := currentState[objPrevRun]; !ok {
				splits := strings.Split(objPrevRun, "#:#")
				l.WithField("role", splits[1]).
					WithField("object_type", splits[0]).
					WithField("object", splits[2]).
					WithField("privilege_type", splits[3]).
					Debug("removed object privileges detected")
				revokeEntry := metrics.NewMeasurement(data.GetEpoch())
				revokeEntry["object_type"] = splits[0]
				revokeEntry["tag_role"] = splits[1]
				revokeEntry["tag_object"] = splits[2]
				revokeEntry["privilege_type"] = splits[3]
				revokeEntry["event"] = "REVOKE"
				detectedChanges = append(detectedChanges, revokeEntry)
				changeCounts.Dropped++
				delete(md.ChangeState["object_privileges"], objPrevRun)
			}
		}
	}

	l.Debugf("object privilege changes detected: %d", len(detectedChanges))
	if len(detectedChanges) > 0 {
		r.measurementCh <- metrics.MeasurementEnvelope{
			DBName:     md.Name,
			MetricName: "privilege_changes",
			Data:       detectedChanges,
			CustomTags: md.CustomTags,
		}
	}

	return changeCounts
}

func (r *Reaper) DetectConfigurationChanges(ctx context.Context, md *sources.SourceConn) ChangeDetectionResults {
	detectedChanges := make(metrics.Measurements, 0)
	var firstRun bool
	var changeCounts ChangeDetectionResults
	l := log.GetLogger(ctx)
	changeCounts.Target = "settings"
	l.Debug("checking for configuration changes...")
	if _, ok := md.ChangeState["configuration_hashes"]; !ok {
		firstRun = true
		md.ChangeState["configuration_hashes"] = make(map[string]string)
	}

	mvp, ok := metricDefs.GetMetricDef("configuration_hashes")
	if !ok {
		l.Error("could not get configuration_hashes sql")
		return changeCounts
	}

	rows, err := md.Conn.Query(ctx, mvp.GetSQL(md.Version))
	if err != nil {
		l.Error(err)
		return changeCounts
	}
	defer rows.Close()
	var (
		objIdent, objValue string
		epoch              int64
	)
	for rows.Next() {
		if rows.Scan(&epoch, &objIdent, &objValue) != nil {
			return changeCounts
		}
		prevРash, ok := md.ChangeState["configuration_hashes"][objIdent]
		ll := l.WithField("setting", objIdent)
		if ok { // we have existing state
			if prevРash != objValue {
				ll.Warningf("settings change detected: %s = %s (prev: %s)", objIdent, objValue, prevРash)
				detectedChanges = append(detectedChanges, metrics.Measurement{
					metrics.EpochColumnName: epoch,
					"tag_setting":           objIdent,
					"value":                 objValue,
					"event":                 "alter"})
				md.ChangeState["configuration_hashes"][objIdent] = objValue
				changeCounts.Altered++
			}
		} else { // check for new, delete not relevant here (pg_upgrade)
			md.ChangeState["configuration_hashes"][objIdent] = objValue
			if firstRun {
				continue
			}
			ll.Debug("new setting detected")
			detectedChanges = append(detectedChanges, metrics.Measurement{
				metrics.EpochColumnName: epoch,
				"tag_setting":           objIdent,
				"value":                 objValue,
				"event":                 "create"})
			changeCounts.Created++
		}
	}

	l.Debugf("configuration changes detected: %d", len(detectedChanges))
	if len(detectedChanges) > 0 {
		r.measurementCh <- metrics.MeasurementEnvelope{
			DBName:     md.Name,
			MetricName: "configuration_changes",
			Data:       detectedChanges,
			CustomTags: md.CustomTags,
		}
	}
	return changeCounts
}

// GetInstanceUpMeasurement returns a single measurement with "instance_up" metric
// used to detect if the instance is up or down
func (r *Reaper) GetInstanceUpMeasurement(ctx context.Context, md *sources.SourceConn) (metrics.Measurements, error) {
	return metrics.Measurements{
		metrics.Measurement{
			metrics.EpochColumnName: time.Now().UnixNano(),
			"instance_up": func() int {
				if md.Conn.Ping(ctx) == nil {
					return 1
				}
				return 0
			}(), // true if connection is up
		},
	}, nil // always return nil error for the status metric
}

func (r *Reaper) GetObjectChangesMeasurement(ctx context.Context, md *sources.SourceConn) (metrics.Measurements, error) {
	md.Lock()
	defer md.Unlock()

	spN := r.DetectSprocChanges(ctx, md)
	tblN := r.DetectTableChanges(ctx, md)
	idxN := r.DetectIndexChanges(ctx, md)
	cnfN := r.DetectConfigurationChanges(ctx, md)
	privN := r.DetectPrivilegeChanges(ctx, md)

	if spN.Total()+tblN.Total()+idxN.Total()+cnfN.Total()+privN.Total() == 0 {
		return nil, nil
	}

	m := metrics.NewMeasurement(time.Now().UnixNano())
	m["details"] = strings.Join([]string{spN.String(), tblN.String(), idxN.String(), cnfN.String(), privN.String()}, " ")
	return metrics.Measurements{m}, nil
}

// Called once on daemon startup if some commonly wanted extension (most notably pg_stat_statements) is missing.
// With newer Postgres version can even succeed if the user is not a real superuser due to some cloud-specific
// whitelisting or "trusted extensions" (a feature from v13). Ignores errors.
func TryCreateMissingExtensions(ctx context.Context, md *sources.SourceConn, extensionNames []string, existingExtensions map[string]int) []string {
	// TODO: move to sources package and use direct pgx connection
	var queryArgs []any
	l := log.GetLogger(ctx)
	extsCreated := make([]string, 0)

	if !md.IsPostgresSource() {
		queryArgs = append(queryArgs, pgx.QueryExecModeSimpleProtocol)
	}

	sqlAvailable := `select name::text from pg_available_extensions`

	// Use direct pgx Query instead of QueryMeasurements wrapper
	rows, err := md.Conn.Query(ctx, sqlAvailable, queryArgs...)
	if err != nil {
		l.Infof("[%s] Failed to get a list of available extensions: %v", md, err)
		return extsCreated
	}
	defer rows.Close()

	// For security reasons don't allow to execute random strings but check that it's an existing extension
	availableExtentions := make(map[string]bool)
	var extentionName string
	for rows.Next() {
		if err := rows.Scan(&extentionName); err != nil {
			l.Infof("[%s] Failed to scan extension name: %v", md, err)
			continue
		}
		availableExtentions[extentionName] = true
	}

	for _, extToCreate := range extensionNames {
		if _, ok := existingExtensions[extToCreate]; ok {
			continue
		}
		_, ok := availableExtentions[extToCreate]
		if !ok {
			log.GetLogger(ctx).Errorf("[%s] Requested extension %s not available on instance, cannot try to create...", md, extToCreate)
		} else {
			sqlCreateExt := `create extension ` + extToCreate
			execArgs := []any{}
			if !md.IsPostgresSource() {
				execArgs = append(execArgs, pgx.QueryExecModeSimpleProtocol)
			}

			if _, err := md.Conn.Exec(ctx, sqlCreateExt, execArgs...); err != nil {
				l.Errorf("[%s] Failed to create extension %s (based on --try-create-listed-exts-if-missing input): %v", md, extToCreate, err)
			} else {
				extsCreated = append(extsCreated, extToCreate)
			}
		}
	}

	return extsCreated
}

// Called once on daemon startup to try to create "metric fething helper" functions automatically
func TryCreateMetricsFetchingHelpers(ctx context.Context, md *sources.SourceConn) (err error) {
	sl := log.GetLogger(ctx).WithField("source", md.Name)
	metrics := maps.Clone(md.Metrics)
	maps.Insert(metrics, maps.All(md.MetricsStandby))
	for metricName := range metrics {
		metric, ok := metricDefs.GetMetricDef(metricName)
		if !ok {
			continue
		}
		if _, err = md.Conn.Exec(ctx, metric.InitSQL); err != nil {
			return
		}
		sl.WithField("metric", metricName).Info("Successfully created metric fetching helpers")
	}
	return
}

func (r *Reaper) CloseResourcesForRemovedMonitoredDBs(hostsToShutDown map[string]bool) {
	for _, prevDB := range r.prevLoopMonitoredDBs {
		if r.monitoredSources.GetMonitoredDatabase(prevDB.Name) == nil { // removed from config
			prevDB.Conn.Close()
			_ = r.SinksWriter.SyncMetric(prevDB.Name, "", sinks.DeleteOp)
		}
	}

	for toShutDownDB := range hostsToShutDown {
		if db := r.monitoredSources.GetMonitoredDatabase(toShutDownDB); db != nil {
			db.Conn.Close()
		}
		_ = r.SinksWriter.SyncMetric(toShutDownDB, "", sinks.DeleteOp)
	}
}
