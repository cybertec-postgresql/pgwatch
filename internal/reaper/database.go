package reaper

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/db"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sinks"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
	"github.com/jackc/pgx/v5"
)

func QueryMeasurements(ctx context.Context, md *sources.SourceConn, sql string, args ...any) (metrics.Measurements, error) {
	var conn db.PgxIface
	var err error
	var tx pgx.Tx
	if strings.TrimSpace(sql) == "" {
		return nil, errors.New("empty SQL")
	}

	conn = md.Conn
	if md.IsPostgresSource() {
		// we don't want transaction for non-postgres sources, e.g. pgbouncer
		if tx, err = conn.Begin(ctx); err != nil {
			return nil, err
		}
		defer func() { _ = tx.Commit(ctx) }()
		_, err = tx.Exec(ctx, "SET LOCAL lock_timeout TO '100ms'")
		if err != nil {
			return nil, err
		}
		conn = tx
	} else {
		// we want simple protocol for non-postgres connections, e.g. pgpool
		args = append([]any{pgx.QueryExecModeSimpleProtocol}, args...)
	}
	rows, err := conn.Query(ctx, sql, args...)
	if err == nil {
		return pgx.CollectRows(rows, metrics.RowToMeasurement)
	}
	return nil, err
}

func (r *Reaper) DetectSprocChanges(ctx context.Context, md *sources.SourceConn) (changeCounts ChangeDetectionResults) {
	detectedChanges := make(metrics.Measurements, 0)
	var firstRun bool
	l := log.GetLogger(ctx)
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
	err := md.Conn.Ping(ctx)
	return metrics.Measurements{
		metrics.Measurement{
			metrics.EpochColumnName: time.Now().UnixNano(),
			"instance_up": func() int {
				if err == nil {
					return 1
				}
				return 0
			}(), // true if connection is up
		},
	}, err
}

func (r *Reaper) CheckForPGObjectChangesAndStore(ctx context.Context, md *sources.SourceConn) {
	md.Lock()
	defer md.Unlock()
	var err error
	l := log.GetLogger(ctx).WithField("source", md.Name).WithField("metric", specialMetricChangeEvents)
	ctx = log.WithLogger(ctx, l)
	sprocCounts := r.DetectSprocChanges(ctx, md) // TODO some of Detect*() code could be unified...
	tableCounts := r.DetectTableChanges(ctx, md)
	indexCounts := r.DetectIndexChanges(ctx, md)
	confCounts := r.DetectConfigurationChanges(ctx, md)
	privChangeCounts := r.DetectPrivilegeChanges(ctx, md)

	// need to send info on all object changes as one message as Grafana applies "last wins" for annotations with similar timestamp
	if sprocCounts.Total() > 0 {
		l = l.WithField("functions", sprocCounts.String())
	}
	if tableCounts.Total() > 0 {
		l = l.WithField("relations", tableCounts.String())
	}
	if indexCounts.Total() > 0 {
		l = l.WithField("indexes", indexCounts.String())
	}
	if confCounts.Total() > 0 {
		l = l.WithField("settings", confCounts.String())
	}
	if privChangeCounts.Total() > 0 {
		l = l.WithField("privileges", privChangeCounts.String())
	}
	if len(l.Data) > 2 { // source and metric are always there
		m := metrics.NewMeasurement(time.Now().UnixNano())
		if m["details"], err = l.String(); err != nil {
			l.Error(err)
			return
		}
		r.measurementCh <- metrics.MeasurementEnvelope{
			DBName:     md.Name,
			MetricName: "object_changes",
			Data:       metrics.Measurements{m},
			CustomTags: md.CustomTags,
		}
		l.Info("detected changes [created/altered/dropped]")
	}
}

// Called once on daemon startup if some commonly wanted extension (most notably pg_stat_statements) is missing.
// With newer Postgres version can even succeed if the user is not a real superuser due to some cloud-specific
// whitelisting or "trusted extensions" (a feature from v13). Ignores errors.
func TryCreateMissingExtensions(ctx context.Context, md *sources.SourceConn, extensionNames []string, existingExtensions map[string]int) []string {
	// TODO: move to sources package and use direct pgx connection
	sqlAvailable := `select name::text from pg_available_extensions`
	extsCreated := make([]string, 0)

	// For security reasons don't allow to execute random strings but check that it's an existing extension
	data, err := QueryMeasurements(ctx, md, sqlAvailable)
	if err != nil {
		log.GetLogger(ctx).Infof("[%s] Failed to get a list of available extensions: %v", md, err)
		return extsCreated
	}

	availableExts := make(map[string]bool)
	for _, row := range data {
		availableExts[row["name"].(string)] = true
	}

	for _, extToCreate := range extensionNames {
		if _, ok := existingExtensions[extToCreate]; ok {
			continue
		}
		_, ok := availableExts[extToCreate]
		if !ok {
			log.GetLogger(ctx).Errorf("[%s] Requested extension %s not available on instance, cannot try to create...", md, extToCreate)
		} else {
			sqlCreateExt := `create extension ` + extToCreate
			_, err := QueryMeasurements(ctx, md, sqlCreateExt)
			if err != nil {
				log.GetLogger(ctx).Errorf("[%s] Failed to create extension %s (based on --try-create-listed-exts-if-missing input): %v", md, extToCreate, err)
			}
			extsCreated = append(extsCreated, extToCreate)
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

func (r *Reaper) CloseResourcesForRemovedMonitoredDBs(shutDownDueToRoleChange map[string]bool) {
	for _, prevDB := range r.prevLoopMonitoredDBs {
		if r.monitoredSources.GetMonitoredDatabase(prevDB.Name) == nil { // removed from config
			prevDB.Conn.Close()
			_ = r.SinksWriter.SyncMetric(prevDB.Name, "", sinks.DeleteOp)
		}
	}

	for roleChangedDB := range shutDownDueToRoleChange {
		if db := r.monitoredSources.GetMonitoredDatabase(roleChangedDB); db != nil {
			db.Conn.Close()
		}
		_ = r.SinksWriter.SyncMetric(roleChangedDB, "", sinks.DeleteOp)
	}
}
