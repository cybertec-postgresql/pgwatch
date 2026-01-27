package reaper

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestStateStore(t *testing.T) {
	tempDir := t.TempDir()
	stateFile := filepath.Join(tempDir, "state.json")

	t.Run("save and load offset", func(t *testing.T) {
		ss, err := NewStateStore(stateFile)
		require.NoError(t, err)

		// Save offset
		err = ss.SetOffset("/var/log/postgresql-00.log", 1234, 5678)
		require.NoError(t, err)

		// Load in new instance
		ss2, err := NewStateStore(stateFile)
		require.NoError(t, err)

		// Verify
		offset := ss2.GetOffset("/var/log/postgresql-00.log")
		assert.Equal(t, int64(1234), offset)
	})

	t.Run("multiple files", func(t *testing.T) {
		ss, err := NewStateStore(stateFile)
		require.NoError(t, err)

		ss.SetOffset("file1.log", 100, 1000)
		ss.SetOffset("file2.log", 200, 2000)
		ss.SetOffset("file3.log", 300, 3000)

		assert.Equal(t, int64(100), ss.GetOffset("file1.log"))
		assert.Equal(t, int64(200), ss.GetOffset("file2.log"))
		assert.Equal(t, int64(300), ss.GetOffset("file3.log"))
	})

	t.Run("non-existent file returns 0", func(t *testing.T) {
		ss, err := NewStateStore(stateFile)
		require.NoError(t, err)

		offset := ss.GetOffset("nonexistent.log")
		assert.Equal(t, int64(0), offset)
	})

	t.Run("state persists across restarts", func(t *testing.T) {
		// First instance
		ss1, err := NewStateStore(stateFile)
		require.NoError(t, err)
		ss1.SetOffset("test.log", 999, 9999)

		// Second instance (simulates restart)
		ss2, err := NewStateStore(stateFile)
		require.NoError(t, err)

		offset := ss2.GetOffset("test.log")
		assert.Equal(t, int64(999), offset)
	})
}

func TestStateStoreAtomicWrite(t *testing.T) {
	tempDir := t.TempDir()
	stateFile := filepath.Join(tempDir, "state.json")

	ss, err := NewStateStore(stateFile)
	require.NoError(t, err)

	// Write multiple times quickly
	for i := 0; i < 10; i++ {
		err := ss.SetOffset("test.log", int64(i*100), int64(i*1000))
		require.NoError(t, err)
	}

	// Verify temp file was cleaned up
	tempFile := stateFile + ".tmp"
	_, err = os.Stat(tempFile)
	assert.True(t, os.IsNotExist(err), "Temp file should not exist after save")

	// Verify final state
	offset := ss.GetOffset("test.log")
	assert.Equal(t, int64(900), offset)
}
