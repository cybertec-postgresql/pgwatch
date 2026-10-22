package reaper

import (
	"context"
	"math/big"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v7/pkg/metrics"
	"github.com/cybertec-postgresql/pgwatch/v7/pkg/sources"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
)

func TestNormalizePoolerMeasurements(t *testing.T) {
	ctx := context.Background()

	t.Run("pgbouncer show stats", func(t *testing.T) {
		data := metrics.Measurements{{
			metrics.EpochColumnName: int64(42),
			"database":              "pgbouncer",
			"total_query_count":     pgtype.Numeric{Int: big.NewInt(1234), Valid: true},
			"avg_query_time":        pgtype.Numeric{Int: big.NewInt(15), Exp: -1, Valid: true},
			"total_wait_time":       pgtype.Numeric{},
			"total_sent":            int64(7),
		}}
		normalizePoolerMeasurements(ctx, sources.SourcePgBouncer, "pgbouncer_stats", data)
		assert.Equal(t, map[string]any{
			metrics.EpochColumnName: int64(42),
			"tag_database":          "pgbouncer",
			"total_query_count":     float64(1234),
			"avg_query_time":        1.5,
			"total_sent":            float64(7),
		}, data[0])
	})

	t.Run("pgpool show pool_nodes", func(t *testing.T) {
		data := metrics.Measurements{{
			"node_id":            "0",
			"hostname":           "pg1",
			"port":               "5432",
			"status":             "up",
			"role":               "primary",
			"lb_weight":          "0.500000",
			"load_balance_node":  "true",
			"select_cnt":         "17",
			"replication_delay":  "0",
			"last_status_change": "2026-10-15 10:00:00",
			"replication_state":  nil,
		}}
		normalizePoolerMeasurements(ctx, sources.SourcePgPool, "pgpool_stats", data)
		assert.Equal(t, map[string]any{
			"tag_node_id":       "0",
			"tag_hostname":      "pg1",
			"tag_port":          "5432",
			"tag_status":        "up",
			"tag_role":          "primary",
			"lb_weight":         0.5,
			"load_balance_node": float64(1),
			"select_cnt":        float64(17),
			"replication_delay": float64(0),
		}, data[0])
	})

	t.Run("postgres source untouched", func(t *testing.T) {
		data := metrics.Measurements{{"database": "x", "query": "select 1"}}
		normalizePoolerMeasurements(ctx, sources.SourcePostgres, "m", data)
		assert.Equal(t, map[string]any{"database": "x", "query": "select 1"}, data[0])
	})
}

func TestToFloat(t *testing.T) {
	for _, tc := range []struct {
		in   any
		want float64
		ok   bool
	}{
		{true, 1, true},
		{"false", 0, true},
		{int32(3), 3, true},
		{"2.5", 2.5, true},
		{"NaN", 0, false},
		{"Inf", 0, false},
		{"up", 0, false},
		{nil, 0, false},
		{pgtype.Numeric{NaN: true, Valid: true}, 0, false},
		{[]byte("1"), 0, false},
	} {
		got, ok := toFloat(tc.in)
		assert.Equal(t, tc.ok, ok, "%v", tc.in)
		assert.Equal(t, tc.want, got, "%v", tc.in)
	}
}
