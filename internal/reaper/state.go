package reaper

import (
	"encoding/json"
	"os"
	"sync"
)

// FileState tracks parsing position for a log file
type FileState struct {
	Filename   string `json:"filename"`
	LastOffset int64  `json:"last_offset"`
	LastSize   int64  `json:"last_size"`
}

// StateStore persists file states across restarts
type StateStore struct {
	filePath string
	states   map[string]*FileState
	mu       sync.RWMutex
}

// NewStateStore creates a new state store
// filePath is where the state JSON file will be saved
func NewStateStore(filePath string) (*StateStore, error) {
	ss := &StateStore{
		filePath: filePath,
		states:   make(map[string]*FileState),
	}

	// Try to load existing state
	if err := ss.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return ss, nil
}

// GetOffset returns the last known offset for a file
func (ss *StateStore) GetOffset(filename string) int64 {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	if state, ok := ss.states[filename]; ok {
		return state.LastOffset
	}
	return 0
}

// SetOffset updates the offset for a file and saves to disk
func (ss *StateStore) SetOffset(filename string, offset, size int64) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	ss.states[filename] = &FileState{
		Filename:   filename,
		LastOffset: offset,
		LastSize:   size,
	}

	return ss.save()
}

// load reads state from disk
func (ss *StateStore) load() error {
	data, err := os.ReadFile(ss.filePath)
	if err != nil {
		return err
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()

	return json.Unmarshal(data, &ss.states)
}

// save writes state to disk atomically
func (ss *StateStore) save() error {
	data, err := json.MarshalIndent(ss.states, "", "  ")
	if err != nil {
		return err
	}

	// Atomic write: write to temp file, then rename
	tempFile := ss.filePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return err
	}

	return os.Rename(tempFile, ss.filePath)
}
