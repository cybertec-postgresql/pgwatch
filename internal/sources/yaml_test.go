package sources_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
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
