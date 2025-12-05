package reaper

import (
	"context"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/testutil"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTryCreateMetricsFetchingHelpers(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	assert.NoError(t, err)
	defer mock.Close()

	metricDefs.MetricDefs["metric1"] = metrics.Metric{
		InitSQL: "CREATE FUNCTION metric1",
	}

	md := &sources.SourceConn{
		Conn: mock,
		Source: sources.Source{
			Name:           "testdb",
			Metrics:        map[string]float64{"metric1": 42, "nonexistent": 0},
			MetricsStandby: map[string]float64{"metric1": 42},
		},
	}

	t.Run("success", func(t *testing.T) {
		mock.ExpectExec("CREATE FUNCTION metric1").WillReturnResult(pgxmock.NewResult("CREATE", 1))

		err = TryCreateMetricsFetchingHelpers(ctx, md)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error on exec", func(t *testing.T) {
		mock.ExpectExec("CREATE FUNCTION metric1").WillReturnError(assert.AnError)

		err = TryCreateMetricsFetchingHelpers(ctx, md)
		assert.Error(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

}

func TestDetectSprocChanges(t *testing.T) {
	ctx := context.Background()

	// Set up simple test metric (instead of complex real one)
	metricDefs.MetricDefs["sproc_hashes"] = metrics.Metric{
		SQLs: map[int]string{
			120000: "SELECT",
		},
	}

	// Create single connection and reaper to maintain state across calls
	md, mock, err := testutil.CreateTestSourceConn()
	assert.NoError(t, err)
	defer mock.Close()

	reaper := &Reaper{
		measurementCh: make(chan metrics.MeasurementEnvelope, 10),
	}

	// First run - establish baseline
	initialRows := pgxmock.NewRows([]string{"tag_sproc", "tag_oid", "md5", "epoch_ns"}).
		AddRow("func1", "123", "hash1", time.Now().UnixNano()).
		AddRow("func2", "456", "hash2", time.Now().UnixNano())
	testutil.ExpectTransaction(mock, initialRows)

	result := reaper.DetectSprocChanges(ctx, md)
	assert.Equal(t, 0, result.Created) // First run should not count anything as created
	assert.Equal(t, 0, result.Altered)
	assert.Equal(t, 0, result.Dropped)

	// State should now be populated
	assert.NotEmpty(t, md.ChangeState["sproc_hashes"])

	// Second run - detect altered sproc (different hash for func1)
	modifiedRows := pgxmock.NewRows([]string{"tag_sproc", "tag_oid", "md5", "epoch_ns"}).
		AddRow("func1", "123", "new_hash", time.Now().UnixNano()).
		AddRow("func2", "456", "hash2", time.Now().UnixNano())
	testutil.ExpectTransaction(mock, modifiedRows)

	result = reaper.DetectSprocChanges(ctx, md)
	assert.Equal(t, 0, result.Created)
	assert.Equal(t, 1, result.Altered) // func1 was altered
	assert.Equal(t, 0, result.Dropped)

	// Third run - detect new sproc (func3 added)
	newSprocRows := pgxmock.NewRows([]string{"tag_sproc", "tag_oid", "md5", "epoch_ns"}).
		AddRow("func1", "123", "new_hash", time.Now().UnixNano()).
		AddRow("func2", "456", "hash2", time.Now().UnixNano()).
		AddRow("func3", "789", "hash3", time.Now().UnixNano()) // new sproc
	testutil.ExpectTransaction(mock, newSprocRows)

	result = reaper.DetectSprocChanges(ctx, md)
	assert.Equal(t, 1, result.Created) // func3 was created
	assert.Equal(t, 0, result.Altered)
	assert.Equal(t, 0, result.Dropped)

	// Verify measurement is sent
	select {
	case <-reaper.measurementCh:
		// Good, measurement was sent
	default:
		t.Error("Expected measurement to be sent")
	}

	// Fourth run - detect dropped sproc (func2 removed)
	droppedSprocRows := pgxmock.NewRows([]string{"tag_sproc", "tag_oid", "md5", "epoch_ns"}).
		AddRow("func1", "123", "new_hash", time.Now().UnixNano()).
		AddRow("func3", "789", "hash3", time.Now().UnixNano()) // func2 dropped
	testutil.ExpectTransaction(mock, droppedSprocRows)

	result = reaper.DetectSprocChanges(ctx, md)
	assert.Equal(t, 0, result.Created)
	assert.Equal(t, 0, result.Altered)
	assert.Equal(t, 1, result.Dropped) // func2 was dropped

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDetectTableChanges(t *testing.T) {
	ctx := context.Background()

	// Set up simple test metric
	metricDefs.MetricDefs["table_hashes"] = metrics.Metric{
		SQLs: map[int]string{
			120000: "SELECT",
		},
	}

	// Create single connection and reaper to maintain state across calls
	md, mock, err := testutil.CreateTestSourceConn()
	assert.NoError(t, err)
	defer mock.Close()

	reaper := &Reaper{
		measurementCh: make(chan metrics.MeasurementEnvelope, 10),
	}

	// First run - establish baseline
	initialRows := pgxmock.NewRows([]string{"tag_table", "tag_oid", "md5", "epoch_ns"}).
		AddRow("table1", "123", "hash1", time.Now().UnixNano()).
		AddRow("table2", "456", "hash2", time.Now().UnixNano())
	testutil.ExpectTransaction(mock, initialRows)

	result := reaper.DetectTableChanges(ctx, md)
	assert.Equal(t, 0, result.Created) // First run should not count anything as created
	assert.Equal(t, 0, result.Altered)
	assert.Equal(t, 0, result.Dropped)
	assert.NotEmpty(t, md.ChangeState["table_hashes"])

	// Second run - detect altered table (different hash for table1)
	modifiedRows := pgxmock.NewRows([]string{"tag_table", "tag_oid", "md5", "epoch_ns"}).
		AddRow("table1", "123", "new_hash", time.Now().UnixNano()).
		AddRow("table2", "456", "hash2", time.Now().UnixNano())
	testutil.ExpectTransaction(mock, modifiedRows)

	result = reaper.DetectTableChanges(ctx, md)
	assert.Equal(t, 0, result.Created)
	assert.Equal(t, 1, result.Altered) // table1 was altered
	assert.Equal(t, 0, result.Dropped)

	// Third run - detect new table (table3 added)
	newTableRows := pgxmock.NewRows([]string{"tag_table", "tag_oid", "md5", "epoch_ns"}).
		AddRow("table1", "123", "new_hash", time.Now().UnixNano()).
		AddRow("table2", "456", "hash2", time.Now().UnixNano()).
		AddRow("table3", "789", "hash3", time.Now().UnixNano()) // new table
	testutil.ExpectTransaction(mock, newTableRows)

	result = reaper.DetectTableChanges(ctx, md)
	assert.Equal(t, 1, result.Created) // table3 was created
	assert.Equal(t, 0, result.Altered)
	assert.Equal(t, 0, result.Dropped)

	// Verify measurement is sent
	select {
	case msg := <-reaper.measurementCh:
		assert.Equal(t, "table_changes", msg.MetricName)
		assert.Equal(t, "testdb", msg.DBName)
	default:
		t.Error("Expected measurement to be sent")
	}

	// Fourth run - detect dropped table (table2 removed)
	droppedTableRows := pgxmock.NewRows([]string{"tag_table", "tag_oid", "md5", "epoch_ns"}).
		AddRow("table1", "123", "new_hash", time.Now().UnixNano()).
		AddRow("table3", "789", "hash3", time.Now().UnixNano()) // table2 dropped
	testutil.ExpectTransaction(mock, droppedTableRows)

	result = reaper.DetectTableChanges(ctx, md)
	assert.Equal(t, 0, result.Created)
	assert.Equal(t, 0, result.Altered)
	assert.Equal(t, 1, result.Dropped) // table2 was dropped

	// Check that table2 was removed from state
	_, exists := md.ChangeState["table_hashes"]["table2"]
	assert.False(t, exists)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDetectIndexChanges(t *testing.T) {
	ctx := context.Background()

	// Set up simple test metric
	metricDefs.MetricDefs["index_hashes"] = metrics.Metric{
		SQLs: map[int]string{
			120000: "SELECT",
		},
	}

	// Create single connection and reaper to maintain state across calls
	md, mock, err := testutil.CreateTestSourceConn()
	assert.NoError(t, err)

	reaper := &Reaper{
		measurementCh: make(chan metrics.MeasurementEnvelope, 10),
	}

	// First run - establish baseline
	initialRows := pgxmock.NewRows([]string{"tag_index", "table", "md5", "is_valid", "epoch_ns"}).
		AddRow("idx1", "table1", "hash1", "t", time.Now().UnixNano()).
		AddRow("idx2", "table1", "hash2", "t", time.Now().UnixNano())
	testutil.ExpectTransaction(mock, initialRows)

	result := reaper.DetectIndexChanges(ctx, md)
	assert.Equal(t, 0, result.Created) // First run should not count anything as created
	assert.Equal(t, 0, result.Altered)
	assert.Equal(t, 0, result.Dropped)
	assert.NotEmpty(t, md.ChangeState["index_hashes"])

	// Second run - detect altered index (is_valid changed for idx1)
	modifiedRows := pgxmock.NewRows([]string{"tag_index", "table", "md5", "is_valid", "epoch_ns"}).
		AddRow("idx1", "table1", "hash1", "f", time.Now().UnixNano()). // now invalid
		AddRow("idx2", "table1", "hash2", "t", time.Now().UnixNano())
	testutil.ExpectTransaction(mock, modifiedRows)

	result = reaper.DetectIndexChanges(ctx, md)
	assert.Equal(t, 0, result.Created)
	assert.Equal(t, 1, result.Altered) // idx1 was altered
	assert.Equal(t, 0, result.Dropped)

	// Third run - detect new index (idx3 added)
	newIndexRows := pgxmock.NewRows([]string{"tag_index", "table", "md5", "is_valid", "epoch_ns"}).
		AddRow("idx1", "table1", "hash1", "f", time.Now().UnixNano()).
		AddRow("idx2", "table1", "hash2", "t", time.Now().UnixNano()).
		AddRow("idx3", "table2", "hash3", "t", time.Now().UnixNano()) // new index
	testutil.ExpectTransaction(mock, newIndexRows)

	result = reaper.DetectIndexChanges(ctx, md)
	assert.Equal(t, 1, result.Created) // idx3 was created
	assert.Equal(t, 0, result.Altered)
	assert.Equal(t, 0, result.Dropped)

	// Verify measurement is sent
	select {
	case msg := <-reaper.measurementCh:
		assert.Equal(t, "index_changes", msg.MetricName)
		assert.Equal(t, "testdb", msg.DBName)
	default:
		t.Error("Expected measurement to be sent")
	}

	// Fourth run - detect dropped index (idx2 removed)
	droppedIndexRows := pgxmock.NewRows([]string{"tag_index", "table", "md5", "is_valid", "epoch_ns"}).
		AddRow("idx1", "table1", "hash1", "f", time.Now().UnixNano()).
		AddRow("idx3", "table2", "hash3", "t", time.Now().UnixNano()) // idx2 dropped
	testutil.ExpectTransaction(mock, droppedIndexRows)

	result = reaper.DetectIndexChanges(ctx, md)
	assert.Equal(t, 0, result.Created)
	assert.Equal(t, 0, result.Altered)
	assert.Equal(t, 1, result.Dropped) // idx2 was dropped

	// Check that idx2 was removed from state
	_, exists := md.ChangeState["index_hashes"]["idx2"]
	assert.False(t, exists)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDetectPrivilegeChanges(t *testing.T) {
	ctx := context.Background()

	// Set up simple test metric
	metricDefs.MetricDefs["privilege_changes"] = metrics.Metric{
		SQLs: map[int]string{
			120000: "SELECT",
		},
	}

	// Create single connection and reaper to maintain state across calls
	md, mock, err := testutil.CreateTestSourceConn()
	assert.NoError(t, err)

	reaper := &Reaper{
		measurementCh: make(chan metrics.MeasurementEnvelope, 10),
	}

	// First run - establish baseline
	initialRows := pgxmock.NewRows([]string{"object_type", "tag_role", "tag_object", "privilege_type", "epoch_ns"}).
		AddRow("table", "user1", "table1", "SELECT", time.Now().UnixNano()).
		AddRow("table", "user2", "table2", "INSERT", time.Now().UnixNano())
	testutil.ExpectTransaction(mock, initialRows)

	result := reaper.DetectPrivilegeChanges(ctx, md)
	assert.Equal(t, 0, result.Created) // First run should not count anything as created
	assert.Equal(t, 0, result.Altered)
	assert.Equal(t, 0, result.Dropped)
	assert.NotEmpty(t, md.ChangeState["object_privileges"])

	// Second run - detect new privilege grant (user1 gets INSERT privilege on table1)
	newPrivilegeRows := pgxmock.NewRows([]string{"object_type", "tag_role", "tag_object", "privilege_type", "epoch_ns"}).
		AddRow("table", "user1", "table1", "SELECT", time.Now().UnixNano()).
		AddRow("table", "user1", "table1", "INSERT", time.Now().UnixNano()). // new privilege
		AddRow("table", "user2", "table2", "INSERT", time.Now().UnixNano())
	testutil.ExpectTransaction(mock, newPrivilegeRows)

	result = reaper.DetectPrivilegeChanges(ctx, md)
	assert.Equal(t, 1, result.Created) // new privilege was granted
	assert.Equal(t, 0, result.Altered)
	assert.Equal(t, 0, result.Dropped)

	// Verify measurement is sent
	select {
	case msg := <-reaper.measurementCh:
		assert.Equal(t, "privilege_changes", msg.MetricName)
		assert.Equal(t, "testdb", msg.DBName)
	default:
		t.Error("Expected measurement to be sent")
	}

	// Third run - detect privilege revoke (user1 loses INSERT privilege on table1)
	revokedPrivilegeRows := pgxmock.NewRows([]string{"object_type", "tag_role", "tag_object", "privilege_type", "epoch_ns"}).
		AddRow("table", "user1", "table1", "SELECT", time.Now().UnixNano()).
		AddRow("table", "user2", "table2", "INSERT", time.Now().UnixNano()) // user1 INSERT privilege revoked
	testutil.ExpectTransaction(mock, revokedPrivilegeRows)

	result = reaper.DetectPrivilegeChanges(ctx, md)
	assert.Equal(t, 0, result.Created)
	assert.Equal(t, 0, result.Altered)
	assert.Equal(t, 1, result.Dropped) // privilege was revoked

	// Check that revoked privilege was removed from state
	_, exists := md.ChangeState["object_privileges"]["table#:#user1#:#table1#:#INSERT"]
	assert.False(t, exists)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDetectConfigurationChanges(t *testing.T) {
	ctx := context.Background()

	// Set up simple test metric
	metricDefs.MetricDefs["configuration_hashes"] = metrics.Metric{
		SQLs: map[int]string{
			120000: "SELECT",
		},
	}

	// Create direct mock connection without transaction expectations
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	md := &sources.SourceConn{
		Conn:   mock,
		Source: sources.Source{Name: "testdb"},
		RuntimeInfo: sources.RuntimeInfo{
			Version:     120000,
			ChangeState: make(map[string]map[string]string),
		},
	}

	reaper := &Reaper{
		measurementCh: make(chan metrics.MeasurementEnvelope, 10),
	}

	// First run - establish baseline configuration
	initialRows := pgxmock.NewRows([]string{"epoch_ns", "setting", "value"}).
		AddRow(time.Now().UnixNano(), "max_connections", "100").
		AddRow(time.Now().UnixNano(), "shared_buffers", "128MB")
	mock.ExpectQuery("SELECT").WillReturnRows(initialRows)

	result := reaper.DetectConfigurationChanges(ctx, md)
	assert.Equal(t, 0, result.Created) // First run should not count anything as created
	assert.Equal(t, 0, result.Altered)
	assert.Equal(t, 0, result.Dropped)
	assert.NotEmpty(t, md.ChangeState["configuration_hashes"])

	// Second run - detect new configuration setting
	newSettingRows := pgxmock.NewRows([]string{"epoch_ns", "setting", "value"}).
		AddRow(time.Now().UnixNano(), "max_connections", "100").
		AddRow(time.Now().UnixNano(), "shared_buffers", "128MB").
		AddRow(time.Now().UnixNano(), "work_mem", "4MB") // new setting
	mock.ExpectQuery("SELECT").WillReturnRows(newSettingRows)

	result = reaper.DetectConfigurationChanges(ctx, md)
	assert.Equal(t, 1, result.Created) // new setting was added
	assert.Equal(t, 0, result.Altered)
	assert.Equal(t, 0, result.Dropped)

	// Verify measurement is sent
	select {
	case msg := <-reaper.measurementCh:
		assert.Equal(t, "configuration_changes", msg.MetricName)
		assert.Equal(t, "testdb", msg.DBName)
	default:
		t.Error("Expected measurement to be sent")
	}

	// Third run - detect configuration change
	changedValueRows := pgxmock.NewRows([]string{"epoch_ns", "setting", "value"}).
		AddRow(time.Now().UnixNano(), "max_connections", "200").  // changed value
		AddRow(time.Now().UnixNano(), "shared_buffers", "256MB"). // changed value
		AddRow(time.Now().UnixNano(), "work_mem", "4MB")
	mock.ExpectQuery("SELECT").WillReturnRows(changedValueRows)

	result = reaper.DetectConfigurationChanges(ctx, md)
	assert.Equal(t, 0, result.Created)
	assert.Equal(t, 2, result.Altered) // two settings were changed
	assert.Equal(t, 0, result.Dropped)

	// Check that state was updated
	assert.Equal(t, "200", md.ChangeState["configuration_hashes"]["max_connections"])
	assert.Equal(t, "256MB", md.ChangeState["configuration_hashes"]["shared_buffers"])

	assert.NoError(t, mock.ExpectationsWereMet())
}
