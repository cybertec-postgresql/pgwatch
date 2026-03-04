package reaper

import "hash/fnv"

// GetPathUnderlyingDeviceID on Darwin falls back to an FNV hash of the path.
// Identical paths still get the same ID, so deduplication works correctly.
func GetPathUnderlyingDeviceID(p string) (uint64, error) {
	h := fnv.New64a()
	_, _ = h.Write([]byte(p))
	return h.Sum64(), nil
}
