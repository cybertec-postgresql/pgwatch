package reaper

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/cmdopts"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockReader struct {
	toReturn sources.Sources
	toErr    error
}

func (m *mockReader) GetSources() (sources.Sources, error) {
	if m.toErr != nil {
		return nil, m.toErr
	}
	return m.toReturn, nil
}

func (m *mockReader) WriteSources(sources.Sources) error { return nil }
func (m *mockReader) DeleteSource(string) error          { return nil }
func (m *mockReader) UpdateSource(sources.Source) error  { return nil }

func TestReaper_LoadSources(t *testing.T) {
	ctx := log.WithLogger(context.Background(), log.NewNoopLogger())

	t.Run("Test pause trigger file", func(t *testing.T) {
		pausefile := filepath.Join(t.TempDir(), "pausefile")
		require.NoError(t, os.WriteFile(pausefile, []byte("foo"), 0644))
		r := NewReaper(ctx, &cmdopts.Options{Metrics: metrics.CmdOpts{EmergencyPauseTriggerfile: pausefile}})
		assert.NoError(t, r.LoadSources())
		assert.True(t, len(r.monitoredSources) == 0, "Expected no monitored sources when pause trigger file exists")
	})

	t.Run("Test SyncFromReader errror", func(t *testing.T) {
		reader := &mockReader{toErr: assert.AnError}
		r := NewReaper(ctx, &cmdopts.Options{SourcesReaderWriter: reader})
		assert.Error(t, r.LoadSources())
		assert.Equal(t, 0, len(r.monitoredSources), "Expected no monitored sources after error")
	})

	t.Run("Test SyncFromReader success", func(t *testing.T) {
		source1 := sources.Source{Name: "Source 1", IsEnabled: true, Kind: sources.SourcePostgres}
		source2 := sources.Source{Name: "Source 2", IsEnabled: true, Kind: sources.SourcePostgres}
		reader := &mockReader{toReturn: sources.Sources{source1, source2}}
		r := NewReaper(ctx, &cmdopts.Options{SourcesReaderWriter: reader})
		assert.NoError(t, r.LoadSources())
		assert.Equal(t, 2, len(r.monitoredSources), "Expected two monitored sources after successful load")
		assert.NotNil(t, r.monitoredSources.GetMonitoredDatabase(source1.Name))
		assert.NotNil(t, r.monitoredSources.GetMonitoredDatabase(source2.Name))
	})

	t.Run("Test repeated load", func(t *testing.T) {
		source1 := sources.Source{Name: "Source 1", IsEnabled: true, Kind: sources.SourcePostgres}
		source2 := sources.Source{Name: "Source 2", IsEnabled: true, Kind: sources.SourcePostgres}
		reader := &mockReader{toReturn: sources.Sources{source1, source2}}
		r := NewReaper(ctx, &cmdopts.Options{SourcesReaderWriter: reader})
		assert.NoError(t, r.LoadSources())
		assert.Equal(t, 2, len(r.monitoredSources), "Expected two monitored sources after first load")

		// Load again with the same sources
		assert.NoError(t, r.LoadSources())
		assert.Equal(t, 2, len(r.monitoredSources), "Expected still two monitored sources after second load")
	})

	t.Run("Test group limited sources", func(t *testing.T) {
		source1 := sources.Source{Name: "Source 1", IsEnabled: true, Kind: sources.SourcePostgres, Group: ""} // Empty group should not filter
		source2 := sources.Source{Name: "Source 2", IsEnabled: true, Kind: sources.SourcePostgres, Group: "group1"}
		source3 := sources.Source{Name: "Source 3", IsEnabled: true, Kind: sources.SourcePostgres, Group: "group2"}
		reader := &mockReader{toReturn: sources.Sources{source1, source2, source3}}
		r := NewReaper(ctx, &cmdopts.Options{SourcesReaderWriter: reader, Sources: sources.CmdOpts{Groups: []string{"group1", "group2"}}})
		assert.NoError(t, r.LoadSources())
		assert.Equal(t, 3, len(r.monitoredSources), "Expected three monitored sources after load")

		r.Sources.Groups = []string{"group1"}
		assert.NoError(t, r.LoadSources())
		assert.Equal(t, 2, len(r.monitoredSources), "Expected two monitored sources after group filtering")
	})
}
