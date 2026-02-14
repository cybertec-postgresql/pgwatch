package sources_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

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

func TestConcurrentSourceUpdates(t *testing.T) {
	a := assert.New(t)
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "sources.yaml")

	yamlrw, err := sources.NewYAMLSourcesReaderWriter(ctx, tempFile)
	a.NoError(err)

	initialSources := sources.Sources{}
	err = yamlrw.WriteSources(initialSources)
	a.NoError(err)

	numGoroutines := 10
	var wg sync.WaitGroup
	errorChan := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			sourceName := fmt.Sprintf("source_%d", id)
			testSource := sources.Source{
				Name:    sourceName,
				ConnStr: fmt.Sprintf("postgresql://localhost/db_%d", id),
				Kind:    sources.SourcePostgres,
			}

			time.Sleep(time.Millisecond * time.Duration(id%3))

			err := yamlrw.UpdateSource(testSource)
			if err != nil {
				errorChan <- fmt.Errorf("goroutine %d: %w", id, err)
			}
		}(i)
	}

	wg.Wait()
	close(errorChan)

	for err := range errorChan {
		t.Logf("Error during concurrent update: %v", err)
	}

	finalSources, err := yamlrw.GetSources()
	a.NoError(err)

	a.Equal(numGoroutines, len(finalSources),
		"Expected %d sources, but got %d. Some updates were lost due to race condition!",
		numGoroutines, len(finalSources))

	// Print missing if test fails
	if len(finalSources) != numGoroutines {
		for i := 0; i < numGoroutines; i++ {
			sourceName := fmt.Sprintf("source_%d", i)
			found := false
			for _, src := range finalSources {
				if src.Name == sourceName {
					found = true
					break
				}
			}
			if !found {
				t.Logf("LOST UPDATE: %s was not saved", sourceName)
			}
		}
	}
}

func TestConcurrentReadsDuringSourceWrites(t *testing.T) {
	a := assert.New(t)
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "sources.yaml")

	yamlrw, err := sources.NewYAMLSourcesReaderWriter(ctx, tempFile)
	a.NoError(err)

	err = yamlrw.WriteSources(sources.Sources{})
	a.NoError(err)

	var wg sync.WaitGroup
	stopReading := make(chan struct{})

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			for {
				select {
				case <-stopReading:
					return
				default:
					_, err := yamlrw.GetSources()
					if err != nil {
						t.Logf("Reader %d error: %v", readerID, err)
					}
					time.Sleep(time.Millisecond)
				}
			}
		}(i)
	}

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			err := yamlrw.UpdateSource(sources.Source{
				Name:    fmt.Sprintf("source_%d", id),
				ConnStr: fmt.Sprintf("postgresql://localhost/db_%d", id),
				Kind:    sources.SourcePostgres,
			})
			if err != nil {
				t.Logf("Error updating source_%d: %v", id, err)
			}
		}(i)
	}

	time.Sleep(100 * time.Millisecond)
	close(stopReading)
	wg.Wait()

	finalSources, err := yamlrw.GetSources()
	a.NoError(err)
	a.Equal(20, len(finalSources), "Expected 20 sources")
}
