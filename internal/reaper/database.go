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
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
	"github.com/jackc/pgx/v5"
)

func QueryMeasurements(ctx context.Context, dbUnique string, sql string, args ...any) (metrics.Measurements, error) {
	// TODO: move to sources package and use direct pgx connection
	var conn db.PgxIface
	var md *sources.SourceConn
	var err error
	var tx pgx.Tx
	if strings.TrimSpace(sql) == "" {
		return nil, errors.New("empty SQL")
	}
	if md, err = GetMonitoredDatabaseByUniqueName(ctx, dbUnique); err != nil {
		return nil, err
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

func DetectSprocChanges(ctx context.Context, md *sources.SourceConn, storageCh chan<- metrics.MeasurementEnvelope, hostState map[string]map[string]string) ChangeDetectionResults {
	detectedChanges := make(metrics.Measurements, 0)
	var firstRun bool
	var changeCounts ChangeDetectionResults

	log.GetLogger(ctx).Debugf("[%s][%s] checking for sproc changes...", md.Name, specialMetricChangeEvents)
	if _, ok := hostState["sproc_hashes"]; !ok {
		firstRun = true
		hostState["sproc_hashes"] = make(map[string]string)
	}

	mvp, ok := metricDefs.GetMetricDef("sproc_hashes")
	if !ok {
		log.GetLogger(ctx).Error("could not get sproc_hashes sql")
		return changeCounts
	}

	data, err := QueryMeasurements(ctx, md.Name, mvp.GetSQL(int(md.Version)))
	if err != nil {
		log.GetLogger(ctx).Error("could not read sproc_hashes from monitored host: ", md.Name, ", err:", err)
		return changeCounts
	}

	for _, dr := range data {
		objIdent := dr["tag_sproc"].(string) + dbMetricJoinStr + dr["tag_oid"].(string)
		prevHash, ok := hostState["sproc_hashes"][objIdent]
		if ok { // we have existing state
			if prevHash != dr["md5"].(string) {
				log.GetLogger(ctx).Info("detected change in sproc:", dr["tag_sproc"], ", oid:", dr["tag_oid"])
				dr["event"] = "alter"
				detectedChanges = append(detectedChanges, dr)
				hostState["sproc_hashes"][objIdent] = dr["md5"].(string)
				changeCounts.Altered++
			}
		} else { // check for new / delete
			if !firstRun {
				log.GetLogger(ctx).Info("detected new sproc:", dr["tag_sproc"], ", oid:", dr["tag_oid"])
				dr["event"] = "create"
				detectedChanges = append(detectedChanges, dr)
				changeCounts.Created++
			}
			hostState["sproc_hashes"][objIdent] = dr["md5"].(string)
		}
	}
	// detect deletes
	if !firstRun && len(hostState["sproc_hashes"]) != len(data) {
		deletedSProcs := make([]string, 0)
		// turn resultset to map => [oid]=true for faster checks
		currentOidMap := make(map[string]bool)
		for _, dr := range data {
			currentOidMap[dr["tag_sproc"].(string)+dbMetricJoinStr+dr["tag_oid"].(string)] = true
		}
		for sprocIdent := range hostState["sproc_hashes"] {
			_, ok := currentOidMap[sprocIdent]
			if !ok {
				splits := strings.Split(sprocIdent, dbMetricJoinStr)
				log.GetLogger(ctx).Info("detected delete of sproc:", splits[0], ", oid:", splits[1])
				influxEntry := metrics.NewMeasurement(data.GetEpoch())
				influxEntry["event"] = "drop"
				influxEntry["tag_sproc"] = splits[0]
				influxEntry["tag_oid"] = splits[1]
				detectedChanges = append(detectedChanges, influxEntry)
				deletedSProcs = append(deletedSProcs, sprocIdent)
				changeCounts.Dropped++
			}
		}
		for _, deletedSProc := range deletedSProcs {
			delete(hostState["sproc_hashes"], deletedSProc)
		}
	}
	log.GetLogger(ctx).Debugf("[%s][%s] detected %d sproc changes", md.Name, specialMetricChangeEvents, len(detectedChanges))
	if len(detectedChanges) > 0 {
		storageCh <- metrics.MeasurementEnvelope{
			DBName:     md.Name,
			MetricName: "sproc_changes",
			Data:       detectedChanges,
			CustomTags: md.CustomTags,
		}
	}

	return changeCounts
}

func DetectTableChanges(ctx context.Context, md *sources.SourceConn, storageCh chan<- metrics.MeasurementEnvelope, hostState map[string]map[string]string) ChangeDetectionResults {
	detectedChanges := make(metrics.Measurements, 0)
	var firstRun bool
	var changeCounts ChangeDetectionResults

	log.GetLogger(ctx).Debugf("[%s][%s] checking for table changes...", md.Name, specialMetricChangeEvents)
	if _, ok := hostState["table_hashes"]; !ok {
		firstRun = true
		hostState["table_hashes"] = make(map[string]string)
	}

	mvp, ok := metricDefs.GetMetricDef("table_hashes")
	if !ok {
		log.GetLogger(ctx).Error("could not get table_hashes sql")
		return changeCounts
	}

	data, err := QueryMeasurements(ctx, md.Name, mvp.GetSQL(int(md.Version)))
	if err != nil {
		log.GetLogger(ctx).Error("could not read table_hashes from monitored host:", md.Name, ", err:", err)
		return changeCounts
	}

	for _, dr := range data {
		objIdent := dr["tag_table"].(string)
		prevHash, ok := hostState["table_hashes"][objIdent]
		//log.Debug("inspecting table:", objIdent, "hash:", prev_hash)
		if ok { // we have existing state
			if prevHash != dr["md5"].(string) {
				log.GetLogger(ctx).Info("detected DDL change in table:", dr["tag_table"])
				dr["event"] = "alter"
				detectedChanges = append(detectedChanges, dr)
				hostState["table_hashes"][objIdent] = dr["md5"].(string)
				changeCounts.Altered++
			}
		} else { // check for new / delete
			if !firstRun {
				log.GetLogger(ctx).Info("detected new table:", dr["tag_table"])
				dr["event"] = "create"
				detectedChanges = append(detectedChanges, dr)
				changeCounts.Created++
			}
			hostState["table_hashes"][objIdent] = dr["md5"].(string)
		}
	}
	// detect deletes
	if !firstRun && len(hostState["table_hashes"]) != len(data) {
		deletedTables := make([]string, 0)
		// turn resultset to map => [table]=true for faster checks
		currentTableMap := make(map[string]bool)
		for _, dr := range data {
			currentTableMap[dr["tag_table"].(string)] = true
		}
		for table := range hostState["table_hashes"] {
			_, ok := currentTableMap[table]
			if !ok {
				log.GetLogger(ctx).Info("detected drop of table:", table)
				influxEntry := metrics.NewMeasurement(data.GetEpoch())
				influxEntry["event"] = "drop"
				influxEntry["tag_table"] = table
				detectedChanges = append(detectedChanges, influxEntry)
				deletedTables = append(deletedTables, table)
				changeCounts.Dropped++
			}
		}
		for _, deletedTable := range deletedTables {
			delete(hostState["table_hashes"], deletedTable)
		}
	}

	log.GetLogger(ctx).Debugf("[%s][%s] detected %d table changes", md.Name, specialMetricChangeEvents, len(detectedChanges))
	if len(detectedChanges) > 0 {
		storageCh <- metrics.MeasurementEnvelope{
			DBName:     md.Name,
			MetricName: "table_changes",
			Data:       detectedChanges,
			CustomTags: md.CustomTags,
		}
	}

	return changeCounts
}

func DetectIndexChanges(ctx context.Context, md *sources.SourceConn, storageCh chan<- metrics.MeasurementEnvelope, hostState map[string]map[string]string) ChangeDetectionResults {
	detectedChanges := make(metrics.Measurements, 0)
	var firstRun bool
	var changeCounts ChangeDetectionResults

	log.GetLogger(ctx).Debugf("[%s][%s] checking for index changes...", md.Name, specialMetricChangeEvents)
	if _, ok := hostState["index_hashes"]; !ok {
		firstRun = true
		hostState["index_hashes"] = make(map[string]string)
	}

	mvp, ok := metricDefs.GetMetricDef("index_hashes")
	if !ok {
		log.GetLogger(ctx).Error("could not get index_hashes sql")
		return changeCounts
	}

	data, err := QueryMeasurements(ctx, md.Name, mvp.GetSQL(int(md.Version)))
	if err != nil {
		log.GetLogger(ctx).Error("could not read index_hashes from monitored host:", md.Name, ", err:", err)
		return changeCounts
	}

	for _, dr := range data {
		objIdent := dr["tag_index"].(string)
		prevHash, ok := hostState["index_hashes"][objIdent]
		if ok { // we have existing state
			if prevHash != (dr["md5"].(string) + dr["is_valid"].(string)) {
				log.GetLogger(ctx).Info("detected index change:", dr["tag_index"], ", table:", dr["table"])
				dr["event"] = "alter"
				detectedChanges = append(detectedChanges, dr)
				hostState["index_hashes"][objIdent] = dr["md5"].(string) + dr["is_valid"].(string)
				changeCounts.Altered++
			}
		} else { // check for new / delete
			if !firstRun {
				log.GetLogger(ctx).Info("detected new index:", dr["tag_index"])
				dr["event"] = "create"
				detectedChanges = append(detectedChanges, dr)
				changeCounts.Created++
			}
			hostState["index_hashes"][objIdent] = dr["md5"].(string) + dr["is_valid"].(string)
		}
	}
	// detect deletes
	if !firstRun && len(hostState["index_hashes"]) != len(data) {
		deletedIndexes := make([]string, 0)
		// turn resultset to map => [table]=true for faster checks
		currentIndexMap := make(map[string]bool)
		for _, dr := range data {
			currentIndexMap[dr["tag_index"].(string)] = true
		}
		for indexName := range hostState["index_hashes"] {
			_, ok := currentIndexMap[indexName]
			if !ok {
				log.GetLogger(ctx).Info("detected drop of index_name:", indexName)
				influxEntry := metrics.NewMeasurement(data.GetEpoch())
				influxEntry["event"] = "drop"
				influxEntry["tag_index"] = indexName
				detectedChanges = append(detectedChanges, influxEntry)
				deletedIndexes = append(deletedIndexes, indexName)
				changeCounts.Dropped++
			}
		}
		for _, deletedIndex := range deletedIndexes {
			delete(hostState["index_hashes"], deletedIndex)
		}
	}
	log.GetLogger(ctx).Debugf("[%s][%s] detected %d index changes", md.Name, specialMetricChangeEvents, len(detectedChanges))
	if len(detectedChanges) > 0 {
		storageCh <- metrics.MeasurementEnvelope{
			DBName:     md.Name,
			MetricName: "index_changes",
			Data:       detectedChanges,
			CustomTags: md.CustomTags,
		}
	}

	return changeCounts
}

func DetectPrivilegeChanges(ctx context.Context, md *sources.SourceConn, storageCh chan<- metrics.MeasurementEnvelope, hostState map[string]map[string]string) ChangeDetectionResults {
	detectedChanges := make(metrics.Measurements, 0)
	var firstRun bool
	var changeCounts ChangeDetectionResults

	log.GetLogger(ctx).Debugf("[%s][%s] checking object privilege changes...", md.Name, specialMetricChangeEvents)
	if _, ok := hostState["object_privileges"]; !ok {
		firstRun = true
		hostState["object_privileges"] = make(map[string]string)
	}

	mvp, ok := metricDefs.GetMetricDef("privilege_changes")
	if !ok || mvp.GetSQL(int(md.Version)) == "" {
		log.GetLogger(ctx).Warningf("[%s][%s] could not get SQL for 'privilege_changes'. cannot detect privilege changes", md.Name, specialMetricChangeEvents)
		return changeCounts
	}

	// returns rows of: object_type, tag_role, tag_object, privilege_type
	data, err := QueryMeasurements(ctx, md.Name, mvp.GetSQL(int(md.Version)))
	if err != nil {
		log.GetLogger(ctx).Errorf("[%s][%s] failed to fetch object privileges info: %v", md.Name, specialMetricChangeEvents, err)
		return changeCounts
	}

	currentState := make(map[string]bool)
	for _, dr := range data {
		objIdent := fmt.Sprintf("%s#:#%s#:#%s#:#%s", dr["object_type"], dr["tag_role"], dr["tag_object"], dr["privilege_type"])
		if firstRun {
			hostState["object_privileges"][objIdent] = ""
		} else {
			_, ok := hostState["object_privileges"][objIdent]
			if !ok {
				log.GetLogger(ctx).Infof("[%s][%s] detected new object privileges: role=%s, object_type=%s, object=%s, privilege_type=%s",
					md.Name, specialMetricChangeEvents, dr["tag_role"], dr["object_type"], dr["tag_object"], dr["privilege_type"])
				dr["event"] = "GRANT"
				detectedChanges = append(detectedChanges, dr)
				changeCounts.Created++
				hostState["object_privileges"][objIdent] = ""
			}
			currentState[objIdent] = true
		}
	}
	// check revokes - exists in old state only
	if !firstRun && len(currentState) > 0 {
		for objPrevRun := range hostState["object_privileges"] {
			if _, ok := currentState[objPrevRun]; !ok {
				splits := strings.Split(objPrevRun, "#:#")
				log.GetLogger(ctx).Infof("[%s][%s] detected removed object privileges: role=%s, object_type=%s, object=%s, privilege_type=%s",
					md.Name, specialMetricChangeEvents, splits[1], splits[0], splits[2], splits[3])
				revokeEntry := metrics.NewMeasurement(data.GetEpoch())
				revokeEntry["object_type"] = splits[0]
				revokeEntry["tag_role"] = splits[1]
				revokeEntry["tag_object"] = splits[2]
				revokeEntry["privilege_type"] = splits[3]
				revokeEntry["event"] = "REVOKE"
				detectedChanges = append(detectedChanges, revokeEntry)
				changeCounts.Dropped++
				delete(hostState["object_privileges"], objPrevRun)
			}
		}
	}

	log.GetLogger(ctx).Debugf("[%s][%s] detected %d object privilege changes...", md.Name, specialMetricChangeEvents, len(detectedChanges))
	if len(detectedChanges) > 0 {
		storageCh <- metrics.MeasurementEnvelope{
			DBName:     md.Name,
			MetricName: "privilege_changes",
			Data:       detectedChanges,
			CustomTags: md.CustomTags,
		}
	}

	return changeCounts
}

func DetectConfigurationChanges(ctx context.Context, md *sources.SourceConn, storageCh chan<- metrics.MeasurementEnvelope, hostState map[string]map[string]string) ChangeDetectionResults {
	detectedChanges := make(metrics.Measurements, 0)
	var firstRun bool
	var changeCounts ChangeDetectionResults

	log.GetLogger(ctx).Debugf("[%s][%s] checking for configuration changes...", md.Name, specialMetricChangeEvents)
	if _, ok := hostState["configuration_hashes"]; !ok {
		firstRun = true
		hostState["configuration_hashes"] = make(map[string]string)
	}

	mvp, ok := metricDefs.GetMetricDef("configuration_hashes")
	if !ok {
		log.GetLogger(ctx).Errorf("[%s][%s] could not get configuration_hashes sql", md.Name, specialMetricChangeEvents)
		return changeCounts
	}

	data, err := QueryMeasurements(ctx, md.Name, mvp.GetSQL(int(md.Version)))
	if err != nil {
		log.GetLogger(ctx).Errorf("[%s][%s] could not read configuration_hashes from monitored host: %v", md.Name, specialMetricChangeEvents, err)
		return changeCounts
	}

	for _, dr := range data {
		objIdent := dr["tag_setting"].(string)
		objValue := dr["value"].(string)
		prevРash, ok := hostState["configuration_hashes"][objIdent]
		if ok { // we have existing state
			if prevРash != objValue {
				if objIdent == "connection_ID" {
					continue // ignore some weird Azure managed PG service setting
				}
				log.GetLogger(ctx).Warningf("[%s][%s] detected settings change: %s = %s (prev: %s)",
					md.Name, specialMetricChangeEvents, objIdent, objValue, prevРash)
				dr["event"] = "alter"
				detectedChanges = append(detectedChanges, dr)
				hostState["configuration_hashes"][objIdent] = objValue
				changeCounts.Altered++
			}
		} else { // check for new, delete not relevant here (pg_upgrade)
			if !firstRun {
				log.GetLogger(ctx).Warningf("[%s][%s] detected new setting: %s", md.Name, specialMetricChangeEvents, objIdent)
				dr["event"] = "create"
				detectedChanges = append(detectedChanges, dr)
				changeCounts.Created++
			}
			hostState["configuration_hashes"][objIdent] = objValue
		}
	}

	log.GetLogger(ctx).Debugf("[%s][%s] detected %d configuration changes", md.Name, specialMetricChangeEvents, len(detectedChanges))
	if len(detectedChanges) > 0 {
		storageCh <- metrics.MeasurementEnvelope{
			DBName:     md.Name,
			MetricName: "configuration_changes",
			Data:       detectedChanges,
			CustomTags: md.CustomTags,
		}
	}

	return changeCounts
}

func (r *Reaper) CheckForPGObjectChangesAndStore(ctx context.Context, dbUnique string, md *sources.SourceConn, hostState map[string]map[string]string) {
	storageCh := r.measurementCh
	sprocCounts := DetectSprocChanges(ctx, md, storageCh, hostState) // TODO some of Detect*() code could be unified...
	tableCounts := DetectTableChanges(ctx, md, storageCh, hostState)
	indexCounts := DetectIndexChanges(ctx, md, storageCh, hostState)
	confCounts := DetectConfigurationChanges(ctx, md, storageCh, hostState)
	privChangeCounts := DetectPrivilegeChanges(ctx, md, storageCh, hostState)

	// need to send info on all object changes as one message as Grafana applies "last wins" for annotations with similar timestamp
	message := ""
	if sprocCounts.Altered > 0 || sprocCounts.Created > 0 || sprocCounts.Dropped > 0 {
		message += fmt.Sprintf(" sprocs %d/%d/%d", sprocCounts.Created, sprocCounts.Altered, sprocCounts.Dropped)
	}
	if tableCounts.Altered > 0 || tableCounts.Created > 0 || tableCounts.Dropped > 0 {
		message += fmt.Sprintf(" tables/views %d/%d/%d", tableCounts.Created, tableCounts.Altered, tableCounts.Dropped)
	}
	if indexCounts.Altered > 0 || indexCounts.Created > 0 || indexCounts.Dropped > 0 {
		message += fmt.Sprintf(" indexes %d/%d/%d", indexCounts.Created, indexCounts.Altered, indexCounts.Dropped)
	}
	if confCounts.Altered > 0 || confCounts.Created > 0 {
		message += fmt.Sprintf(" configuration %d/%d/%d", confCounts.Created, confCounts.Altered, confCounts.Dropped)
	}
	if privChangeCounts.Dropped > 0 || privChangeCounts.Created > 0 {
		message += fmt.Sprintf(" privileges %d/%d/%d", privChangeCounts.Created, privChangeCounts.Altered, privChangeCounts.Dropped)
	}

	if message > "" {
		message = "Detected changes for \"" + dbUnique + "\" [Created/Altered/Dropped]:" + message
		log.GetLogger(ctx).Info(message)
		detectedChangesSummary := make(metrics.Measurements, 0)
		influxEntry := metrics.NewMeasurement(time.Now().UnixNano())
		influxEntry["details"] = message
		detectedChangesSummary = append(detectedChangesSummary, influxEntry)
		md, _ := GetMonitoredDatabaseByUniqueName(ctx, dbUnique)
		storageCh <- metrics.MeasurementEnvelope{
			DBName:     dbUnique,
			SourceType: string(md.Kind),
			MetricName: "object_changes",
			Data:       detectedChangesSummary,
			CustomTags: md.CustomTags,
		}

	}
}

// Called once on daemon startup if some commonly wanted extension (most notably pg_stat_statements) is missing.
// With newer Postgres version can even succeed if the user is not a real superuser due to some cloud-specific
// whitelisting or "trusted extensions" (a feature from v13). Ignores errors.
func TryCreateMissingExtensions(ctx context.Context, dbUnique string, extensionNames []string, existingExtensions map[string]int) []string {
	// TODO: move to sources package and use direct pgx connection
	sqlAvailable := `select name::text from pg_available_extensions`
	extsCreated := make([]string, 0)

	// For security reasons don't allow to execute random strings but check that it's an existing extension
	data, err := QueryMeasurements(ctx, dbUnique, sqlAvailable)
	if err != nil {
		log.GetLogger(ctx).Infof("[%s] Failed to get a list of available extensions: %v", dbUnique, err)
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
			log.GetLogger(ctx).Errorf("[%s] Requested extension %s not available on instance, cannot try to create...", dbUnique, extToCreate)
		} else {
			sqlCreateExt := `create extension ` + extToCreate
			_, err := QueryMeasurements(ctx, dbUnique, sqlCreateExt)
			if err != nil {
				log.GetLogger(ctx).Errorf("[%s] Failed to create extension %s (based on --try-create-listed-exts-if-missing input): %v", dbUnique, extToCreate, err)
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
			_ = r.SinksWriter.SyncMetric(prevDB.Name, "", "remove")
		}
	}

	for roleChangedDB := range shutDownDueToRoleChange {
		if db := r.monitoredSources.GetMonitoredDatabase(roleChangedDB); db != nil {
			db.Conn.Close()
		}
		_ = r.SinksWriter.SyncMetric(roleChangedDB, "", "remove")
	}
}
