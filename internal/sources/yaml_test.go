package sources_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
	"github.com/stretchr/testify/assert"
)

// the number of entries in the sample.sources.yaml file
const sampleEntriesNumber = 4

const (
	contribDir = "../../contrib/"
	sampleFile = "../../contrib/sample.sources.yaml"
)

func TestNewYAMLSourcesReaderWriter(t *testing.T) {
	a := assert.New(t)
	yamlrw, err := sources.NewYAMLSourcesReaderWriter(ctx, sampleFile)
	a.NoError(err)
	a.NotNil(t, yamlrw)
}

func TestYAMLGetMonitoredDatabases(t *testing.T) {
	a := assert.New(t)

	t.Run("single file", func(*testing.T) {
		yamlrw, err := sources.NewYAMLSourcesReaderWriter(ctx, sampleFile)
		a.NoError(err)

		dbs, err := yamlrw.GetSources()
		a.NoError(err)
		a.Len(dbs, sampleEntriesNumber)
	})

	t.Run("nonexistent file", func(*testing.T) {
		yamlrw, err := sources.NewYAMLSourcesReaderWriter(ctx, "nonexistent.yaml")
		a.NoError(err)
		dbs, err := yamlrw.GetSources()
		a.Error(err)
		a.Nil(dbs)
	})

	t.Run("garbage file", func(*testing.T) {
		yamlrw, err := sources.NewYAMLSourcesReaderWriter(ctx, filepath.Join(contribDir, "yaml.go"))
		a.NoError(err)
		dbs, err := yamlrw.GetSources()
		a.Error(err)
		a.Nil(dbs)
	})

	t.Run("duplicate in single file", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "duplicate.yaml")
		yamlContent := `
- name: test1
  conn_str: postgresql://localhost/test1
- name: test2
  conn_str: postgresql://localhost/test2
- name: test1
  conn_str: postgresql://localhost/test1_duplicate
`
		err := os.WriteFile(tmpFile, []byte(yamlContent), 0644)
		a.NoError(err)
		yamlrw, err := sources.NewYAMLSourcesReaderWriter(ctx, tmpFile)
		a.NoError(err)

		dbs, err := yamlrw.GetSources()
		a.Error(err)
		a.Nil(dbs)
	})

	t.Run("duplicates across files", func(t *testing.T) {
		tmpDir := t.TempDir()
		yamlContent1 := `
- name: test1
  conn_str: postgresql://localhost/test1
- name: test2
  conn_str: postgresql://localhost/test2
`
		err := os.WriteFile(filepath.Join(tmpDir, "sources1.yaml"), []byte(yamlContent1), 0644)
		a.NoError(err)

		yamlContent2 := `
- name: test1
  conn_str: postgresql://localhost/test1_duplicate
`
		err = os.WriteFile(filepath.Join(tmpDir, "sources2.yaml"), []byte(yamlContent2), 0644)
		a.NoError(err)
		yamlrw, err := sources.NewYAMLSourcesReaderWriter(ctx, tmpDir)
		a.NoError(err)

		dbs, err := yamlrw.GetSources()
		a.Error(err)
		a.Nil(dbs)
	})
}

func TestYAMLDeleteDatabase(t *testing.T) {
	a := assert.New(t)

	t.Run("happy path", func(*testing.T) {
		data, err := os.ReadFile(sampleFile)
		a.NoError(err)
		tmpSampleFile := filepath.Join(t.TempDir(), "sample.sources.yaml")
		err = os.WriteFile(tmpSampleFile, data, 0644)
		a.NoError(err)
		defer os.Remove(tmpSampleFile)

		yamlrw, err := sources.NewYAMLSourcesReaderWriter(ctx, tmpSampleFile)
		a.NoError(err)

		err = yamlrw.DeleteSource("test1")
		a.NoError(err)

		dbs, err := yamlrw.GetSources()
		a.NoError(err)
		a.Len(dbs, sampleEntriesNumber-1)
	})

	t.Run("nonexistent file", func(*testing.T) {
		yamlrw, err := sources.NewYAMLSourcesReaderWriter(ctx, "nonexistent.yaml")
		a.NoError(err)
		err = yamlrw.DeleteSource("test1")
		a.Error(err)
	})
}

func TestYAMLUpdateDatabase(t *testing.T) {
	a := assert.New(t)

	t.Run("happy path", func(*testing.T) {
		data, err := os.ReadFile(sampleFile)
		a.NoError(err)
		tmpSampleFile := filepath.Join(t.TempDir(), "sample.sources.yaml")
		err = os.WriteFile(tmpSampleFile, data, 0644)
		a.NoError(err)
		defer os.Remove(tmpSampleFile)

		yamlrw, err := sources.NewYAMLSourcesReaderWriter(ctx, tmpSampleFile)
		a.NoError(err)

		// change the connection string of the first database
		md := sources.Source{}
		md.Name = "test1"
		md.ConnStr = "postgresql://localhost/test1"
		err = yamlrw.UpdateSource(md)
		a.NoError(err)

		// add a new database
		md = sources.Source{}
		md.Name = "test5"
		md.ConnStr = "postgresql://localhost/test5"
		err = yamlrw.UpdateSource(md)
		a.NoError(err)

		dbs, err := yamlrw.GetSources()
		a.NoError(err)
		a.Len(dbs, sampleEntriesNumber+1)
		dbs[0].ConnStr = "postgresql://localhost/test1"
		dbs[sampleEntriesNumber].ConnStr = "postgresql://localhost/test5"
	})

	t.Run("nonexistent file", func(*testing.T) {
		yamlrw, err := sources.NewYAMLSourcesReaderWriter(ctx, "")
		a.NoError(err)
		err = yamlrw.UpdateSource(sources.Source{})
		a.Error(err)
	})
}

func TestYAMLCreateSource(t *testing.T) {
	a := assert.New(t)

	t.Run("happy_path", func(*testing.T) {
		data, err := os.ReadFile(sampleFile)
		a.NoError(err)
		tmpSampleFile := filepath.Join(t.TempDir(), "sample.sources.yaml")
		err = os.WriteFile(tmpSampleFile, data, 0644)
		a.NoError(err)
		defer os.Remove(tmpSampleFile)

		yamlrw, err := sources.NewYAMLSourcesReaderWriter(ctx, tmpSampleFile)
		a.NoError(err)

		// Create a new source
		md := sources.Source{
			Name:    "new_source",
			ConnStr: "postgresql://localhost/new_db",
			Kind:    sources.SourcePostgres,
		}
		err = yamlrw.CreateSource(md)
		a.NoError(err)

		// Verify it was created
		dbs, err := yamlrw.GetSources()
		a.NoError(err)
		a.Len(dbs, sampleEntriesNumber+1)

		// Try to create the same source again - should fail
		err = yamlrw.CreateSource(md)
		a.Error(err)
		a.ErrorIs(sources.ErrSourceExists, err)
	})

	t.Run("duplicate_source", func(*testing.T) {
		data, err := os.ReadFile(sampleFile)
		a.NoError(err)
		tmpSampleFile := filepath.Join(t.TempDir(), "sample.sources.yaml")
		err = os.WriteFile(tmpSampleFile, data, 0644)
		a.NoError(err)
		defer os.Remove(tmpSampleFile)

		yamlrw, err := sources.NewYAMLSourcesReaderWriter(ctx, tmpSampleFile)
		a.NoError(err)

		// Try to create a source that already exists
		md := sources.Source{
			Name:    "test1", // This name already exists in sample file
			ConnStr: "postgresql://localhost/test1",
			Kind:    sources.SourcePostgres,
		}
		err = yamlrw.CreateSource(md)
		a.Error(err)
		a.ErrorIs(sources.ErrSourceExists, err)
	})
}

func TestExpandEnvVars(t *testing.T) {
	a := assert.New(t)

	// Set environment variables for testing
	t.Setenv("PGW_TEST_NAME", "expanded_name")
	t.Setenv("PGW_TEST_GROUP", "expanded_group")
	t.Setenv("PGW_TEST_CONNSTR", "postgresql://localhost/expanded")
	t.Setenv("PGW_TEST_KIND", "postgres")
	t.Setenv("PGW_TEST_INCLUDE", "include_pattern")
	t.Setenv("PGW_TEST_EXCLUDE", "exclude_pattern")
	t.Setenv("PGW_TEST_PRESET", "exhaustive")
	t.Setenv("PGW_TEST_PRESET_STANDBY", "standby_preset")

	t.Run("all fields expanded", func(*testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "env_sources.yaml")
		yamlContent := `
- name: $PGW_TEST_NAME
  group: $PGW_TEST_GROUP
  conn_str: $PGW_TEST_CONNSTR
  kind: $PGW_TEST_KIND
  include_pattern: $PGW_TEST_INCLUDE
  exclude_pattern: $PGW_TEST_EXCLUDE
  preset_metrics: $PGW_TEST_PRESET
  preset_metrics_standby: $PGW_TEST_PRESET_STANDBY
`
		err := os.WriteFile(tmpFile, []byte(yamlContent), 0644)
		a.NoError(err)

		yamlrw, err := sources.NewYAMLSourcesReaderWriter(ctx, tmpFile)
		a.NoError(err)

		dbs, err := yamlrw.GetSources()
		a.NoError(err)
		a.Len(dbs, 1)

		src := dbs[0]
		a.Equal("expanded_name", src.Name)
		a.Equal("expanded_group", src.Group)
		a.Equal("postgresql://localhost/expanded", src.ConnStr)
		a.Equal(sources.SourcePostgres, src.Kind)
		a.Equal("include_pattern", src.IncludePattern)
		a.Equal("exclude_pattern", src.ExcludePattern)
		a.Equal("exhaustive", src.PresetMetrics)
		a.Equal("standby_preset", src.PresetMetricsStandby)
	})

	t.Run("no expansion without dollar prefix", func(*testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "no_env_sources.yaml")
		yamlContent := `
- name: literal_name
  group: literal_group
  conn_str: postgresql://localhost/literal
  kind: postgres
  include_pattern: literal_include
  exclude_pattern: literal_exclude
  preset_metrics: basic
  preset_metrics_standby: basic_standby
`
		err := os.WriteFile(tmpFile, []byte(yamlContent), 0644)
		a.NoError(err)

		yamlrw, err := sources.NewYAMLSourcesReaderWriter(ctx, tmpFile)
		a.NoError(err)

		dbs, err := yamlrw.GetSources()
		a.NoError(err)
		a.Len(dbs, 1)

		src := dbs[0]
		a.Equal("literal_name", src.Name)
		a.Equal("literal_group", src.Group)
		a.Equal("postgresql://localhost/literal", src.ConnStr)
		a.Equal(sources.SourcePostgres, src.Kind)
		a.Equal("literal_include", src.IncludePattern)
		a.Equal("literal_exclude", src.ExcludePattern)
		a.Equal("basic", src.PresetMetrics)
		a.Equal("basic_standby", src.PresetMetricsStandby)
	})

	t.Run("unset env var expands to empty", func(*testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "unset_env_sources.yaml")
		yamlContent := `
- name: $PGW_UNSET_VAR
  conn_str: postgresql://localhost/test
`
		err := os.WriteFile(tmpFile, []byte(yamlContent), 0644)
		a.NoError(err)

		yamlrw, err := sources.NewYAMLSourcesReaderWriter(ctx, tmpFile)
		a.NoError(err)

		dbs, err := yamlrw.GetSources()
		a.NoError(err)
		a.Len(dbs, 1)
		a.Equal("", dbs[0].Name)
	})
}

func TestConcurrentSourceUpdates(t *testing.T) {
	a := assert.New(t)
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "sources.yaml")

	yamlrw, err := sources.NewYAMLSourcesReaderWriter(ctx, tempFile)
	a.NoError(err)

	err = yamlrw.WriteSources(sources.Sources{})
	a.NoError(err)

	numGoroutines := 10
	var wg sync.WaitGroup

	// Each goroutine will add a unique source
	for id := range numGoroutines {
		wg.Go(func() {
			testSource := sources.Source{
				Name:          fmt.Sprintf("source_%d", id),
				ConnStr:       fmt.Sprintf("postgresql://localhost/test_%d", id),
				Kind:          sources.SourcePostgres,
				PresetMetrics: "basic",
			}
			time.Sleep(time.Millisecond * time.Duration(id%3))
			err := yamlrw.UpdateSource(testSource)
			a.NoError(err, "Error during concurrent update")
		})
	}

	wg.Wait()

	finalSources, err := yamlrw.GetSources()
	a.NoError(err)
	a.Equal(numGoroutines, len(finalSources), "Some updates were lost due to race condition!")
}
