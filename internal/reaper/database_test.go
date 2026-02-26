package reaper

import (
	"context"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/cmdopts"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sinks"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockSinkWriter implements sinks.Writer interface for testing
type MockSinkWriter struct {
	WriteFunc      func(msg metrics.MeasurementEnvelope) error
	SyncMetricFunc func(dbUnique, metricName string, op sinks.SyncOp) error
}

func (m *MockSinkWriter) Write(msg metrics.MeasurementEnvelope) error {
	if m.WriteFunc != nil {
		return m.WriteFunc(msg)
	}
	return nil
}

func (m *MockSinkWriter) SyncMetric(dbUnique, metricName string, op sinks.SyncOp) error {
	if m.SyncMetricFunc != nil {
		return m.SyncMetricFunc(dbUnique, metricName, op)
	}
	return nil
}

// Helper function to create a test SourceConn with pgxmock
func createTestSourceConn(t *testing.T) (*sources.SourceConn, pgxmock.PgxPoolIface) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)

	md := &sources.SourceConn{
		Conn:   mock,
		Source: sources.Source{Name: "testdb"},
		RuntimeInfo: sources.RuntimeInfo{
			Version:     120000,
			ChangeState: make(map[string]map[string]string),
		},
	}
	return md, mock
}

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
	md, mock := createTestSourceConn(t)
	defer mock.Close()

	reaper := &Reaper{
		measurementCh: make(chan metrics.MeasurementEnvelope, 10),
	}

	// First run - establish baseline
	initialRows := pgxmock.NewRows([]string{"tag_sproc", "tag_oid", "md5", "epoch_ns"}).
		AddRow("func1", "123", "hash1", time.Now().UnixNano()).
		AddRow("func2", "456", "hash2", time.Now().UnixNano())
	mock.ExpectQuery("SELECT").WillReturnRows(initialRows)

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
	mock.ExpectQuery("SELECT").WillReturnRows(modifiedRows)

	result = reaper.DetectSprocChanges(ctx, md)
	assert.Equal(t, 0, result.Created)
	assert.Equal(t, 1, result.Altered) // func1 was altered
	assert.Equal(t, 0, result.Dropped)

	// Third run - detect new sproc (func3 added)
	newSprocRows := pgxmock.NewRows([]string{"tag_sproc", "tag_oid", "md5", "epoch_ns"}).
		AddRow("func1", "123", "new_hash", time.Now().UnixNano()).
		AddRow("func2", "456", "hash2", time.Now().UnixNano()).
		AddRow("func3", "789", "hash3", time.Now().UnixNano()) // new sproc
	mock.ExpectQuery("SELECT").WillReturnRows(newSprocRows)

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
	mock.ExpectQuery("SELECT").WillReturnRows(droppedSprocRows)

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
	md, mock := createTestSourceConn(t)
	defer mock.Close()

	reaper := &Reaper{
		measurementCh: make(chan metrics.MeasurementEnvelope, 10),
	}

	// First run - establish baseline
	initialRows := pgxmock.NewRows([]string{"tag_table", "tag_oid", "md5", "epoch_ns"}).
		AddRow("table1", "123", "hash1", time.Now().UnixNano()).
		AddRow("table2", "456", "hash2", time.Now().UnixNano())
	mock.ExpectQuery("SELECT").WillReturnRows(initialRows)

	result := reaper.DetectTableChanges(ctx, md)
	assert.Equal(t, 0, result.Created) // First run should not count anything as created
	assert.Equal(t, 0, result.Altered)
	assert.Equal(t, 0, result.Dropped)
	assert.NotEmpty(t, md.ChangeState["table_hashes"])

	// Second run - detect altered table (different hash for table1)
	modifiedRows := pgxmock.NewRows([]string{"tag_table", "tag_oid", "md5", "epoch_ns"}).
		AddRow("table1", "123", "new_hash", time.Now().UnixNano()).
		AddRow("table2", "456", "hash2", time.Now().UnixNano())
	mock.ExpectQuery("SELECT").WillReturnRows(modifiedRows)

	result = reaper.DetectTableChanges(ctx, md)
	assert.Equal(t, 0, result.Created)
	assert.Equal(t, 1, result.Altered) // table1 was altered
	assert.Equal(t, 0, result.Dropped)

	// Third run - detect new table (table3 added)
	newTableRows := pgxmock.NewRows([]string{"tag_table", "tag_oid", "md5", "epoch_ns"}).
		AddRow("table1", "123", "new_hash", time.Now().UnixNano()).
		AddRow("table2", "456", "hash2", time.Now().UnixNano()).
		AddRow("table3", "789", "hash3", time.Now().UnixNano()) // new table
	mock.ExpectQuery("SELECT").WillReturnRows(newTableRows)

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
	mock.ExpectQuery("SELECT").WillReturnRows(droppedTableRows)

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
	md, mock := createTestSourceConn(t)
	defer mock.Close()

	reaper := &Reaper{
		measurementCh: make(chan metrics.MeasurementEnvelope, 10),
	}

	// First run - establish baseline
	initialRows := pgxmock.NewRows([]string{"tag_index", "table", "md5", "is_valid", "epoch_ns"}).
		AddRow("idx1", "table1", "hash1", "t", time.Now().UnixNano()).
		AddRow("idx2", "table1", "hash2", "t", time.Now().UnixNano())
	mock.ExpectQuery("SELECT").WillReturnRows(initialRows)

	result := reaper.DetectIndexChanges(ctx, md)
	assert.Equal(t, 0, result.Created) // First run should not count anything as created
	assert.Equal(t, 0, result.Altered)
	assert.Equal(t, 0, result.Dropped)
	assert.NotEmpty(t, md.ChangeState["index_hashes"])

	// Second run - detect altered index (is_valid changed for idx1)
	modifiedRows := pgxmock.NewRows([]string{"tag_index", "table", "md5", "is_valid", "epoch_ns"}).
		AddRow("idx1", "table1", "hash1", "f", time.Now().UnixNano()). // now invalid
		AddRow("idx2", "table1", "hash2", "t", time.Now().UnixNano())
	mock.ExpectQuery("SELECT").WillReturnRows(modifiedRows)

	result = reaper.DetectIndexChanges(ctx, md)
	assert.Equal(t, 0, result.Created)
	assert.Equal(t, 1, result.Altered) // idx1 was altered
	assert.Equal(t, 0, result.Dropped)

	// Third run - detect new index (idx3 added)
	newIndexRows := pgxmock.NewRows([]string{"tag_index", "table", "md5", "is_valid", "epoch_ns"}).
		AddRow("idx1", "table1", "hash1", "f", time.Now().UnixNano()).
		AddRow("idx2", "table1", "hash2", "t", time.Now().UnixNano()).
		AddRow("idx3", "table2", "hash3", "t", time.Now().UnixNano()) // new index
	mock.ExpectQuery("SELECT").WillReturnRows(newIndexRows)

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
	mock.ExpectQuery("SELECT").WillReturnRows(droppedIndexRows)

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
	md, mock := createTestSourceConn(t)
	defer mock.Close()

	reaper := &Reaper{
		measurementCh: make(chan metrics.MeasurementEnvelope, 10),
	}

	// First run - establish baseline
	initialRows := pgxmock.NewRows([]string{"object_type", "tag_role", "tag_object", "privilege_type", "epoch_ns"}).
		AddRow("table", "user1", "table1", "SELECT", time.Now().UnixNano()).
		AddRow("table", "user2", "table2", "INSERT", time.Now().UnixNano())
	mock.ExpectQuery("SELECT").WillReturnRows(initialRows)

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
	mock.ExpectQuery("SELECT").WillReturnRows(newPrivilegeRows)

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
	mock.ExpectQuery("SELECT").WillReturnRows(revokedPrivilegeRows)

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

func TestGetInstanceUpMeasurement(t *testing.T) {
	ctx := context.Background()
	reaper := &Reaper{}

	testCases := []struct {
		name            string
		pingError       error
		expectedUpValue int
	}{
		{
			name:            "connection is up",
			pingError:       nil,
			expectedUpValue: 1,
		},
		{
			name:            "connection is down",
			pingError:       assert.AnError,
			expectedUpValue: 0,
		},
		{
			name:            "connection timeout",
			pingError:       context.DeadlineExceeded,
			expectedUpValue: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			md, mock := createTestSourceConn(t)
			defer mock.Close()

			// Setup ping expectation
			if tc.pingError == nil {
				mock.ExpectPing()
			} else {
				mock.ExpectPing().WillReturnError(tc.pingError)
			}

			measurements, err := reaper.GetInstanceUpMeasurement(ctx, md)

			// Should never return an error
			assert.NoError(t, err)
			require.NotNil(t, measurements)
			require.Len(t, measurements, 1)

			// Check instance_up metric value
			measurement := measurements[0]
			assert.Contains(t, measurement, "instance_up")
			assert.Equal(t, tc.expectedUpValue, measurement["instance_up"])

			// Verify epoch is set and valid
			assert.Contains(t, measurement, metrics.EpochColumnName)
			assert.Greater(t, measurement[metrics.EpochColumnName].(int64), int64(0))
			assert.LessOrEqual(t, measurement[metrics.EpochColumnName].(int64), time.Now().UnixNano())

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestTryCreateMissingExtensions(t *testing.T) {
	ctx := context.Background()

	t.Run("extension already exists", func(t *testing.T) {
		md, mock := createTestSourceConn(t)
		defer mock.Close()

		md.Extensions = map[string]int{"pg_stat_statements": 1}

		mock.ExpectQuery("select name::text from pg_available_extensions").
			WillReturnRows(pgxmock.NewRows([]string{"name"}).
				AddRow("pg_stat_statements").
				AddRow("pgcrypto"))

		extsCreated := TryCreateMissingExtensions(ctx, md, []string{"pg_stat_statements"}, md.Extensions)
		assert.Empty(t, extsCreated, "Should not create extension that already exists")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("extension not available", func(t *testing.T) {
		md, mock := createTestSourceConn(t)
		defer mock.Close()

		mock.ExpectQuery("select name::text from pg_available_extensions").
			WillReturnRows(pgxmock.NewRows([]string{"name"}).AddRow("pgcrypto"))

		extsCreated := TryCreateMissingExtensions(ctx, md, []string{"nonexistent_ext"}, md.Extensions)
		assert.Empty(t, extsCreated, "Should not create extension that is not available")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("extension created successfully", func(t *testing.T) {
		md, mock := createTestSourceConn(t)
		defer mock.Close()

		mock.ExpectQuery("select name::text from pg_available_extensions").
			WillReturnRows(pgxmock.NewRows([]string{"name"}).AddRow("pgcrypto"))
		mock.ExpectQuery("create extension pgcrypto").
			WillReturnRows(pgxmock.NewRows([]string{"name"}))

		extsCreated := TryCreateMissingExtensions(ctx, md, []string{"pgcrypto"}, md.Extensions)
		assert.Equal(t, []string{"pgcrypto"}, extsCreated)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("extension creation fails", func(t *testing.T) {
		md, mock := createTestSourceConn(t)
		defer mock.Close()

		mock.ExpectQuery("select name::text from pg_available_extensions").
			WillReturnRows(pgxmock.NewRows([]string{"name"}).AddRow("pgcrypto"))
		mock.ExpectQuery("create extension pgcrypto").
			WillReturnError(assert.AnError)

		extsCreated := TryCreateMissingExtensions(ctx, md, []string{"pgcrypto"}, md.Extensions)
		// Note: function still appends to extsCreated even on error, which is a known behavior
		assert.NotEmpty(t, extsCreated, "Extension name is added even on failure (known behavior)")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query available extensions fails", func(t *testing.T) {
		md, mock := createTestSourceConn(t)
		defer mock.Close()

		mock.ExpectQuery("select name::text from pg_available_extensions").
			WillReturnError(assert.AnError)

		extsCreated := TryCreateMissingExtensions(ctx, md, []string{"pgcrypto"}, md.Extensions)
		assert.Empty(t, extsCreated)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGetObjectChangesMeasurement(t *testing.T) {
	ctx := context.Background()

	t.Run("no changes detected", func(t *testing.T) {
		md, mock := createTestSourceConn(t)
		defer mock.Close()

		reaper := &Reaper{
			measurementCh: make(chan metrics.MeasurementEnvelope, 10),
		}

		// Setup all detection functions to return no changes
		metricDefs.MetricDefs["sproc_hashes"] = metrics.Metric{
			SQLs: map[int]string{120000: "SELECT"},
		}
		metricDefs.MetricDefs["table_hashes"] = metrics.Metric{
			SQLs: map[int]string{120000: "SELECT"},
		}
		metricDefs.MetricDefs["index_hashes"] = metrics.Metric{
			SQLs: map[int]string{120000: "SELECT"},
		}
		metricDefs.MetricDefs["configuration_hashes"] = metrics.Metric{
			SQLs: map[int]string{120000: "SELECT"},
		}
		metricDefs.MetricDefs["privilege_changes"] = metrics.Metric{
			SQLs: map[int]string{120000: "SELECT"},
		}

		// All queries return empty results
		for range 5 {
			mock.ExpectQuery("SELECT").
				WillReturnRows(pgxmock.NewRows([]string{"epoch_ns"}))
		}

		measurements, err := reaper.GetObjectChangesMeasurement(ctx, md)
		assert.Nil(t, measurements, "Should return nil when no changes detected")
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("changes detected", func(t *testing.T) {
		md, mock := createTestSourceConn(t)
		defer mock.Close()

		reaper := &Reaper{
			measurementCh: make(chan metrics.MeasurementEnvelope, 10),
		}

		// Setup metric definitions
		metricDefs.MetricDefs["sproc_hashes"] = metrics.Metric{
			SQLs: map[int]string{120000: "SELECT"},
		}
		metricDefs.MetricDefs["table_hashes"] = metrics.Metric{
			SQLs: map[int]string{120000: "SELECT"},
		}
		metricDefs.MetricDefs["index_hashes"] = metrics.Metric{
			SQLs: map[int]string{120000: "SELECT"},
		}
		metricDefs.MetricDefs["configuration_hashes"] = metrics.Metric{
			SQLs: map[int]string{120000: "SELECT"},
		}
		metricDefs.MetricDefs["privilege_changes"] = metrics.Metric{
			SQLs: map[int]string{120000: "SELECT"},
		}

		// First call returns a change (sproc)
		mock.ExpectQuery("SELECT").
			WillReturnRows(pgxmock.NewRows([]string{"tag_sproc", "tag_oid", "md5", "epoch_ns"}).
				AddRow("func1", "123", "hash1", time.Now().UnixNano()))
		// Rest return empty
		for range 4 {
			mock.ExpectQuery("SELECT").
				WillReturnRows(pgxmock.NewRows([]string{"epoch_ns"}))
		}

		measurements, err := reaper.GetObjectChangesMeasurement(ctx, md)
		assert.Nil(t, measurements, "First run should not detect changes")
		assert.NoError(t, err)

		// Second call - detect altered sproc
		mock.ExpectQuery("SELECT").
			WillReturnRows(pgxmock.NewRows([]string{"tag_sproc", "tag_oid", "md5", "epoch_ns"}).
				AddRow("func1", "123", "new_hash", time.Now().UnixNano()))
		for range 4 {
			mock.ExpectQuery("SELECT").
				WillReturnRows(pgxmock.NewRows([]string{"epoch_ns"}))
		}

		measurements, err = reaper.GetObjectChangesMeasurement(ctx, md)
		assert.NotNil(t, measurements, "Should detect changes on second run")
		assert.Len(t, measurements, 1)
		assert.Contains(t, measurements[0], "details")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestCloseResourcesForRemovedMonitoredDBs(t *testing.T) {
	logger := log.NewNoopLogger()

	t.Run("remove DB from config", func(t *testing.T) {
		mockConn, err := pgxmock.NewPool()
		require.NoError(t, err)
		mockConn.ExpectClose()

		prevDB := &sources.SourceConn{
			Source: sources.Source{Name: "removed_db"},
			Conn:   mockConn,
		}
		reaper := &Reaper{
			Options:              &cmdopts.Options{},
			logger:               logger,
			prevLoopMonitoredDBs: sources.SourceConns{prevDB},
			monitoredSources:     sources.SourceConns{}, // DB removed from config
		}

		mockSink := &MockSinkWriter{}
		syncCalled := false
		mockSink.SyncMetricFunc = func(dbUnique, metricName string, op sinks.SyncOp) error {
			syncCalled = true
			assert.Equal(t, "removed_db", dbUnique)
			assert.Equal(t, sinks.DeleteOp, op)
			return nil
		}
		reaper.SinksWriter = mockSink

		reaper.CloseResourcesForRemovedMonitoredDBs(nil)
		assert.True(t, syncCalled, "SyncMetric should be called with DeleteOp")
		assert.NoError(t, mockConn.ExpectationsWereMet())
	})

	t.Run("shutdown DB due to role change", func(t *testing.T) {
		mockConn, err := pgxmock.NewPool()
		require.NoError(t, err)
		mockConn.ExpectClose()

		dbToShutdown := &sources.SourceConn{
			Source: sources.Source{Name: "standby_db"},
			Conn:   mockConn,
		}
		reaper := &Reaper{
			Options:          &cmdopts.Options{},
			logger:           logger,
			monitoredSources: sources.SourceConns{dbToShutdown},
		}

		mockSink := &MockSinkWriter{}
		syncCalled := false
		mockSink.SyncMetricFunc = func(dbUnique, metricName string, op sinks.SyncOp) error {
			syncCalled = true
			assert.Equal(t, "standby_db", dbUnique)
			return nil
		}
		reaper.SinksWriter = mockSink

		hostsToShutDown := map[string]bool{"standby_db": true}
		reaper.CloseResourcesForRemovedMonitoredDBs(hostsToShutDown)
		assert.True(t, syncCalled, "SyncMetric should be called")
		assert.NoError(t, mockConn.ExpectationsWereMet())
	})
}

func TestShutdownOldWorkers(t *testing.T) {
	ctx := context.Background()
	logger := log.NewNoopLogger()
	ctx = log.WithLogger(ctx, logger)

	mockSink := &MockSinkWriter{
		SyncMetricFunc: func(dbUnique, metricName string, op sinks.SyncOp) error {
			return nil
		},
	}

	t.Run("shutdown due to host role change", func(t *testing.T) {
		reaper := &Reaper{
			Options: &cmdopts.Options{
				SinksWriter: mockSink,
			},
			logger:  logger,
			cancelFuncs: map[string]context.CancelFunc{
				"db1¤¤¤cpu": func() {},
			},
			prevLoopMonitoredDBs: sources.SourceConns{},
			monitoredSources:     sources.SourceConns{},
		}

		hostsToShutDown := map[string]bool{"db1": true}
		assert.NotPanics(t, func() {
			reaper.ShutdownOldWorkers(ctx, hostsToShutDown)
		})
		_, exists := reaper.cancelFuncs["db1¤¤¤cpu"]
		assert.False(t, exists, "Cancel func should be removed")
	})

	t.Run("shutdown due to metric removed from preset", func(t *testing.T) {
		md := &sources.SourceConn{
			Source: sources.Source{
				Name:    "db1",
				Metrics: map[string]float64{"cpu": 10}, // cpu removed
			},
		}

		reaper := &Reaper{
			Options: &cmdopts.Options{
				SinksWriter: mockSink,
			},
			logger:  logger,
			cancelFuncs: map[string]context.CancelFunc{
				"db1¤¤¤memory": func() {},
			},
			prevLoopMonitoredDBs: sources.SourceConns{},
			monitoredSources:     sources.SourceConns{md},
		}

		assert.NotPanics(t, func() {
			reaper.ShutdownOldWorkers(ctx, nil)
		})
		_, exists := reaper.cancelFuncs["db1¤¤¤memory"]
		assert.False(t, exists, "Cancel func should be removed for metric not in current config")
	})

	t.Run("shutdown due to interval set to zero", func(t *testing.T) {
		md := &sources.SourceConn{
			Source: sources.Source{
				Name:    "db1",
				Metrics: map[string]float64{"cpu": 0}, // interval set to 0
			},
		}

		reaper := &Reaper{
			Options: &cmdopts.Options{
				SinksWriter: mockSink,
			},
			logger:  logger,
			cancelFuncs: map[string]context.CancelFunc{
				"db1¤¤¤cpu": func() {},
			},
			prevLoopMonitoredDBs: sources.SourceConns{},
			monitoredSources:     sources.SourceConns{md},
		}

		assert.NotPanics(t, func() {
			reaper.ShutdownOldWorkers(ctx, nil)
		})
		_, exists := reaper.cancelFuncs["db1¤¤¤cpu"]
		assert.False(t, exists, "Cancel func should be removed when interval is 0")
	})
}

func TestDetectSprocChanges_NoMetricDef(t *testing.T) {
	ctx := context.Background()
	md, mock := createTestSourceConn(t)
	defer mock.Close()

	reaper := &Reaper{
		measurementCh: make(chan metrics.MeasurementEnvelope, 10),
	}

	// No metric definition for sproc_hashes
	result := reaper.DetectSprocChanges(ctx, md)
	assert.Equal(t, 0, result.Total())
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDetectTableChanges_NoMetricDef(t *testing.T) {
	ctx := context.Background()
	md, mock := createTestSourceConn(t)
	defer mock.Close()

	reaper := &Reaper{
		measurementCh: make(chan metrics.MeasurementEnvelope, 10),
	}

	// No metric definition for table_hashes
	result := reaper.DetectTableChanges(ctx, md)
	assert.Equal(t, 0, result.Total())
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDetectIndexChanges_NoMetricDef(t *testing.T) {
	ctx := context.Background()
	md, mock := createTestSourceConn(t)
	defer mock.Close()

	reaper := &Reaper{
		measurementCh: make(chan metrics.MeasurementEnvelope, 10),
	}

	// No metric definition for index_hashes
	result := reaper.DetectIndexChanges(ctx, md)
	assert.Equal(t, 0, result.Total())
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDetectPrivilegeChanges_NoMetricDef(t *testing.T) {
	ctx := context.Background()
	md, mock := createTestSourceConn(t)
	defer mock.Close()

	reaper := &Reaper{
		measurementCh: make(chan metrics.MeasurementEnvelope, 10),
	}

	// No metric definition for privilege_changes
	result := reaper.DetectPrivilegeChanges(ctx, md)
	assert.Equal(t, 0, result.Total())
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDetectConfigurationChanges_NoMetricDef(t *testing.T) {
	ctx := context.Background()
	md, mock := createTestSourceConn(t)
	defer mock.Close()

	reaper := &Reaper{
		measurementCh: make(chan metrics.MeasurementEnvelope, 10),
	}

	// No metric definition for configuration_hashes
	result := reaper.DetectConfigurationChanges(ctx, md)
	assert.Equal(t, 0, result.Total())
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDetectConfigurationChanges_QueryError(t *testing.T) {
	ctx := context.Background()
	md, mock := createTestSourceConn(t)
	defer mock.Close()

	reaper := &Reaper{
		measurementCh: make(chan metrics.MeasurementEnvelope, 10),
	}

	metricDefs.MetricDefs["configuration_hashes"] = metrics.Metric{
		SQLs: map[int]string{120000: "SELECT"},
	}

	mock.ExpectQuery("SELECT").WillReturnError(assert.AnError)

	result := reaper.DetectConfigurationChanges(ctx, md)
	assert.Equal(t, 0, result.Total())
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDetectPrivilegeChanges_QueryError(t *testing.T) {
	ctx := context.Background()
	md, mock := createTestSourceConn(t)
	defer mock.Close()

	reaper := &Reaper{
		measurementCh: make(chan metrics.MeasurementEnvelope, 10),
	}

	metricDefs.MetricDefs["privilege_changes"] = metrics.Metric{
		SQLs: map[int]string{120000: "SELECT"},
	}

	mock.ExpectQuery("SELECT").WillReturnError(assert.AnError)

	result := reaper.DetectPrivilegeChanges(ctx, md)
	assert.Equal(t, 0, result.Total())
	assert.NoError(t, mock.ExpectationsWereMet())
}
