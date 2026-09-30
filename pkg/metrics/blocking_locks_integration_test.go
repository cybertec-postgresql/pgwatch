package metrics_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v7/internal/testutil"
	"github.com/cybertec-postgresql/pgwatch/v7/pkg/metrics"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blockingLocksHarness runs the embedded blocking_locks SQL against a real
// server. pg_blocking_pids() cannot be mocked, so every scenario below opens
// real sessions and makes one of them wait for a lock.
type blockingLocksHarness struct {
	t       *testing.T
	connStr string
	sql     string
	control *pgx.Conn // runs the metric and pg_stat_activity lookups
}

func setupBlockingLocks(t *testing.T) *blockingLocksHarness {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pgContainer, tearDown, err := testutil.SetupPostgresContainer()
	require.NoError(t, err, "failed to start postgres container")
	t.Cleanup(tearDown)

	connStr, err := pgContainer.ConnectionString(testutil.TestContext, "sslmode=disable")
	require.NoError(t, err)

	m, ok := metrics.GetDefaultMetrics().MetricDefs["blocking_locks"]
	require.True(t, ok, "blocking_locks must exist in metrics.yaml")
	sql, ok := m.SQLs[14]
	require.True(t, ok, "blocking_locks must have SQL for key 14")

	h := &blockingLocksHarness{t: t, connStr: connStr, sql: sql}
	h.control = h.newSession()
	_, err = h.control.Exec(testutil.TestContext, "create table t(id int primary key, v int); insert into t values (1, 0)")
	require.NoError(t, err)
	return h
}

// newSession opens one backend. Closing the connection on cleanup rolls
// back whatever transaction the session left open.
func (h *blockingLocksHarness) newSession() *pgx.Conn {
	h.t.Helper()
	conn, err := pgx.Connect(testutil.TestContext, h.connStr)
	require.NoError(h.t, err)
	h.t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = conn.Close(ctx)
	})
	return conn
}

// holdLock opens a transaction on conn, runs stmt inside it and leaves the
// transaction open so every lock stmt took stays held.
func (h *blockingLocksHarness) holdLock(conn *pgx.Conn, stmt string) {
	h.t.Helper()
	_, err := conn.Exec(testutil.TestContext, "begin")
	require.NoError(h.t, err)
	_, err = conn.Exec(testutil.TestContext, stmt)
	require.NoError(h.t, err)
}

// waitFor runs stmt on conn in the background and returns once the server
// reports that backend waiting for a lock. The statement is cancelled on
// cleanup so the connection can close.
func (h *blockingLocksHarness) waitFor(conn *pgx.Conn, stmt string) {
	h.t.Helper()
	ctx, cancel := context.WithCancel(testutil.TestContext)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = conn.Exec(ctx, stmt)
	}()
	h.t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	})
	pid := conn.PgConn().PID()
	require.Eventually(h.t, func() bool {
		var waiting bool
		err := h.control.QueryRow(testutil.TestContext,
			"select coalesce(wait_event_type = 'Lock', false) from pg_stat_activity where pid = $1", pid).Scan(&waiting)
		return err == nil && waiting
	}, 10*time.Second, 50*time.Millisecond, "backend %d never started waiting for a lock", pid)
}

// runMetric executes the blocking_locks SQL from the control session and
// returns every row keyed by column name.
func (h *blockingLocksHarness) runMetric() []map[string]any {
	h.t.Helper()
	rows, err := h.control.Query(testutil.TestContext, h.sql)
	require.NoError(h.t, err)
	defer rows.Close()
	var out []map[string]any
	fields := rows.FieldDescriptions()
	for rows.Next() {
		vals, err := rows.Values()
		require.NoError(h.t, err)
		row := make(map[string]any, len(vals))
		for i, f := range fields {
			row[f.Name] = vals[i]
		}
		out = append(out, row)
	}
	require.NoError(h.t, rows.Err())
	return out
}

// pidOf normalises the integer pgx hands back for a pid column.
func pidOf(v any) uint32 {
	switch n := v.(type) {
	case int32:
		return uint32(n)
	case int64:
		return uint32(n)
	default:
		return 0
	}
}

func describeRows(rows []map[string]any) string {
	var b strings.Builder
	for _, r := range rows {
		fmt.Fprintf(&b, "waiting_pid=%v other_pid=%v other_locktype=%v other_mode=%v\n",
			r["waiting_pid"], r["other_pid"], r["other_locktype"], r["other_mode"])
	}
	return b.String()
}

// AC-005 / CON-001: no waiting session, no row.
func TestBlockingLocksNoWait(t *testing.T) {
	h := setupBlockingLocks(t)
	a := h.newSession()
	h.holdLock(a, "update t set v = 1 where id = 1")
	assert.Empty(t, h.runMetric())
}

// AC-002: a granted lock that does not conflict with the waiting request is
// not a blocker. A's AccessShareLock does not conflict with CREATE INDEX's
// ShareLock; C's RowExclusiveLock does.
func TestBlockingLocksModeConflict(t *testing.T) {
	h := setupBlockingLocks(t)
	a := h.newSession()
	c := h.newSession()
	b := h.newSession()
	h.holdLock(a, "select * from t")
	h.holdLock(c, "insert into t values (2, 0)")
	h.waitFor(b, "create index on t (v)")

	rows := h.runMetric()
	require.NotEmpty(t, rows, "B waits for C, expected at least one row")
	for _, r := range rows {
		assert.Equal(t, b.PgConn().PID(), pidOf(r["waiting_pid"]), describeRows(rows))
		assert.Equal(t, c.PgConn().PID(), pidOf(r["other_pid"]), "A holds a compatible lock and is not a blocker\n%s", describeRows(rows))
	}
}

// AC-003: advisory locks have no relation and no transaction id, so a join on
// those columns misses them. pg_blocking_pids() does not.
func TestBlockingLocksAdvisory(t *testing.T) {
	h := setupBlockingLocks(t)
	a := h.newSession()
	b := h.newSession()
	_, err := a.Exec(testutil.TestContext, "select pg_advisory_lock(1)")
	require.NoError(t, err)
	h.waitFor(b, "select pg_advisory_lock(1)")

	rows := h.runMetric()
	require.Len(t, rows, 1, describeRows(rows))
	assert.Equal(t, b.PgConn().PID(), pidOf(rows[0]["waiting_pid"]))
	assert.Equal(t, a.PgConn().PID(), pidOf(rows[0]["other_pid"]))
	assert.Equal(t, "advisory", rows[0]["other_locktype"])
	assert.Equal(t, "advisory", rows[0]["tag_waiting_locktype"])
}

// AC-004 / REQ-001: A holds two granted locks on t (AccessShareLock from the
// SELECT, RowExclusiveLock from the INSERT). B waits for the whole table.
// One pair, one row.
func TestBlockingLocksOneRowPerPair(t *testing.T) {
	h := setupBlockingLocks(t)
	a := h.newSession()
	b := h.newSession()
	h.holdLock(a, "select * from t")
	_, err := a.Exec(testutil.TestContext, "insert into t values (2, 0)")
	require.NoError(t, err)
	h.waitFor(b, "begin; lock table t in access exclusive mode")

	rows := h.runMetric()
	require.Len(t, rows, 1, describeRows(rows))
	assert.Equal(t, b.PgConn().PID(), pidOf(rows[0]["waiting_pid"]))
	assert.Equal(t, a.PgConn().PID(), pidOf(rows[0]["other_pid"]))
	assert.Equal(t, "t", rows[0]["other_table"])
	assert.NotNil(t, rows[0]["other_mode"])
}

// AC-001 / REQ-003: waiting_seconds counts from pg_locks.waitstart.
func TestBlockingLocksWaitingSeconds(t *testing.T) {
	h := setupBlockingLocks(t)
	a := h.newSession()
	b := h.newSession()
	h.holdLock(a, "update t set v = 1 where id = 1")
	h.waitFor(b, "update t set v = 2 where id = 1")
	time.Sleep(2 * time.Second)

	rows := h.runMetric()
	require.Len(t, rows, 1, describeRows(rows))
	assert.Equal(t, b.PgConn().PID(), pidOf(rows[0]["waiting_pid"]))
	assert.Equal(t, a.PgConn().PID(), pidOf(rows[0]["other_pid"]))
	secs, ok := rows[0]["waiting_seconds"].(float64)
	require.True(t, ok, "waiting_seconds must be a float8, got %T", rows[0]["waiting_seconds"])
	assert.GreaterOrEqual(t, secs, 2.0)
	assert.Less(t, secs, 60.0)
	// Rounded to three decimals: scaling by 1000 leaves no fraction.
	scaled := secs * 1000
	assert.InDelta(t, scaled, float64(int64(scaled+0.5)), 1e-6, "waiting_seconds must be rounded to three decimals")
}
