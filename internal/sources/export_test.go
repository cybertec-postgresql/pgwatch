package sources

import "time"

// SetLastCheckedForTesting sets the atomic lastCheckedNs timestamp on a DbConn.
// It is only used in tests to simulate a recently-fetched state without hitting the DB.
func (md *DbConn) SetLastCheckedForTesting(t time.Time) {
	md.lastCheckedNs.Store(t.UnixNano())
}
