package reaper

import (
	"context"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"

	"github.com/cybertec-postgresql/pgwatch/v7/pkg/log"
	"github.com/cybertec-postgresql/pgwatch/v7/pkg/metrics"
	"github.com/cybertec-postgresql/pgwatch/v7/pkg/sources"
	"github.com/jackc/pgx/v5/pgtype"
)

// poolerTagColumns lists SHOW command columns that become tag_ columns, since pooler admin consoles cannot alias them in SQL.
var poolerTagColumns = map[sources.Kind]map[string]struct{}{
	sources.SourcePgBouncer: {
		"database": {}, "user": {}, "type": {}, "state": {}, "addr": {}, "port": {}, "application_name": {},
	},
	sources.SourcePgPool: {
		"node_id": {}, "hostname": {}, "port": {}, "status": {}, "pg_status": {}, "role": {}, "pg_role": {},
		"pool_pid": {}, "database": {}, "username": {},
	},
}

// normalizePoolerMeasurements renames label columns to tag_ text and converts the rest to float64, dropping what cannot be converted.
func normalizePoolerMeasurements(ctx context.Context, kind sources.Kind, metricName string, data metrics.Measurements) {
	tagCols, ok := poolerTagColumns[kind]
	if !ok {
		return
	}
	l := log.GetLogger(ctx)
	for _, m := range data {
		for _, k := range slices.Collect(maps.Keys(m)) { // snapshot: keys added below must not be revisited
			v := m[k]
			if k == metrics.EpochColumnName {
				continue
			}
			if _, isTag := tagCols[k]; isTag {
				delete(m, k)
				if v != nil {
					m[metrics.TagPrefix+k] = fmt.Sprint(v)
				}
				continue
			}
			if f, ok := toFloat(v); ok {
				m[k] = f
				continue
			}
			delete(m, k)
			if v != nil {
				l.WithField("metric", metricName).Debugf("dropping column %s, cannot convert %v to float", k, v)
			}
		}
	}
}

func toFloat(v any) (float64, bool) {
	var f float64
	switch t := v.(type) {
	case nil:
		return 0, false
	case bool:
		if t {
			return 1, true
		}
		return 0, true
	case int:
		f = float64(t)
	case int16:
		f = float64(t)
	case int32:
		f = float64(t)
	case int64:
		f = float64(t)
	case float32:
		f = float64(t)
	case float64:
		f = t
	case pgtype.Numeric:
		v, err := t.Float64Value()
		if err != nil || !v.Valid {
			return 0, false
		}
		f = v.Float64
	case string:
		switch t {
		case "true":
			return 1, true
		case "false":
			return 0, true
		}
		var err error
		if f, err = strconv.ParseFloat(t, 64); err != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}
