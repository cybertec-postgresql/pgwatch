package sources_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cybertec-postgresql/pgwatch3/sources"
)

// the number of entries in the sample.sources.yaml file
const sampleEntriesNumber = 4

var (
	currentDir string
	sampleFile string
)

func init() {
	// setup the test environment
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("Cannot get the current file path")
	}
	currentDir = filepath.Dir(filename)
	sampleFile = filepath.Join(currentDir, "sample.sources.yaml")
}

func TestNewYAMLSourcesReaderWriter(t *testing.T) {
	a := assert.New(t)
	yamlrw, err := sources.NewYAMLSourcesReaderWriter(ctx, "../sample.sources.yaml")
	a.NoError(err)
	a.NotNil(t, yamlrw)
}

func TestYAMLGetMonitoredDatabases(t *testing.T) {
	a := assert.New(t)

	t.Run("single file", func(*testing.T) {
		yamlrw, err := sources.NewYAMLSourcesReaderWriter(ctx, sampleFile)
		a.NoError(err)

		dbs, err := yamlrw.GetMonitoredDatabases()
		a.NoError(err)
		a.Len(dbs, sampleEntriesNumber)
	})

	t.Run("folder with yaml files", func(*testing.T) {
		yamlrw, err := sources.NewYAMLSourcesReaderWriter(ctx, currentDir)
		a.NoError(err)

		dbs, err := yamlrw.GetMonitoredDatabases()
		a.NoError(err)
		a.Len(dbs, sampleEntriesNumber)
	})

	t.Run("nonexistent file", func(*testing.T) {
		yamlrw, err := sources.NewYAMLSourcesReaderWriter(ctx, "nonexistent.yaml")
		a.NoError(err)
		dbs, err := yamlrw.GetMonitoredDatabases()
		a.Error(err)
		a.Nil(dbs)
	})

	t.Run("garbage file", func(*testing.T) {
		yamlrw, err := sources.NewYAMLSourcesReaderWriter(ctx, filepath.Join(currentDir, "yaml.go"))
		a.NoError(err)
		dbs, err := yamlrw.GetMonitoredDatabases()
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

		err = yamlrw.DeleteDatabase("test1")
		a.NoError(err)

		dbs, err := yamlrw.GetMonitoredDatabases()
		a.NoError(err)
		a.Len(dbs, sampleEntriesNumber-1)
	})

	t.Run("nonexistent file", func(*testing.T) {
		yamlrw, err := sources.NewYAMLSourcesReaderWriter(ctx, "nonexistent.yaml")
		a.NoError(err)
		err = yamlrw.DeleteDatabase("test1")
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
		md := &sources.MonitoredDatabase{DBUniqueName: "test1", ConnStr: "postgresql://localhost/test1"}
		err = yamlrw.UpdateDatabase(md)
		a.NoError(err)

		// add a new database
		md = &sources.MonitoredDatabase{DBUniqueName: "test5", ConnStr: "postgresql://localhost/test5"}
		err = yamlrw.UpdateDatabase(md)
		a.NoError(err)

		dbs, err := yamlrw.GetMonitoredDatabases()
		a.NoError(err)
		a.Len(dbs, sampleEntriesNumber+1)
		dbs.GetMonitoredDatabase("test1").ConnStr = "postgresql://localhost/test1"
		dbs.GetMonitoredDatabase("test5").ConnStr = "postgresql://localhost/test5"
	})

	t.Run("nonexistent file", func(*testing.T) {
		yamlrw, err := sources.NewYAMLSourcesReaderWriter(ctx, "")
		a.NoError(err)
		err = yamlrw.UpdateDatabase(&sources.MonitoredDatabase{})
		a.Error(err)
	})
}
