package reaper

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Byte-offset resumption.

func TestOffsetsStartAtEndOfAnExistingFile(t *testing.T) {
	sizes := map[string]int64{"/logs/postgresql.csv": 4096}
	o := newEndSeededOffsets(func(p string) (int64, bool) {
		s, ok := sizes[p]
		return s, ok
	})

	// A file with content this process did not write starts at its end.
	// Counting it would report months of retained logs as one interval.
	off, ok := o.Get("/logs/postgresql.csv")
	assert.True(t, ok)
	assert.Equal(t, int64(4096), off)

	// The seeded offset sticks even if the file grows, so the second look
	// does not skip what arrived in between.
	sizes["/logs/postgresql.csv"] = 9000
	off, ok = o.Get("/logs/postgresql.csv")
	assert.True(t, ok)
	assert.Equal(t, int64(4096), off, "the seed is taken once, not re-taken")
}

func TestOffsetsStartAtZeroForAFileThatDoesNotExistYet(t *testing.T) {
	o := newEndSeededOffsets(func(string) (int64, bool) { return 0, false })

	// A file created after pgwatch started is read from its first byte:
	// everything in it happened on this process's watch.
	off, ok := o.Get("/logs/created-later.csv")
	assert.False(t, ok)
	assert.Zero(t, off)
}

func TestOffsetsRoundTripAByteOffset(t *testing.T) {
	o := newEndSeededOffsets(func(string) (int64, bool) { return 0, false })
	o.Set("/logs/a.csv", 1234)

	off, ok := o.Get("/logs/a.csv")
	require.True(t, ok)
	assert.Equal(t, int64(1234), off)
}

// The bound must not evict the file currently being read.
//
// The pre-migration code bounded its map by clearing ALL of it at the limit,
// which discarded the active file's offset too. That file would then be
// re-seeded to its current end and every record written since would be
// skipped -- a silent gap in the counts, triggered only on a server with
// thousands of rotated logs.
func TestOffsetBoundEvictsTheOldestNotTheActiveFile(t *testing.T) {
	o := newEndSeededOffsets(func(string) (int64, bool) { return 0, false })

	active := "/logs/active.csv"
	o.Set(active, 100)

	// Push the map well past its bound with files that are never read
	// again, touching the active file as we go, as a real run would.
	for i := range maxTrackedFiles * 2 {
		o.Set(fmt.Sprintf("/logs/rotated-%d.csv", i), int64(i))
		o.Set(active, int64(200+i))
	}

	o.mu.Lock()
	size := len(o.seen)
	o.mu.Unlock()
	assert.LessOrEqual(t, size, maxTrackedFiles, "the map stays bounded")

	off, ok := o.Get(active)
	assert.True(t, ok, "the active file must survive eviction")
	assert.Equal(t, int64(200+maxTrackedFiles*2-1), off)

	// The oldest rotated file is the one that went.
	_, ok = o.Get("/logs/rotated-0.csv")
	assert.False(t, ok, "the least recently used entry is evicted")
}
