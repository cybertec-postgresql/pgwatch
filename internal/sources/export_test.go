package sources

import "time"

// SetLastCheckedForTesting sets the atomic lastCheckedNs timestamp on a DbConn.
// It is only used in tests to simulate a recently-fetched state without hitting the DB.
func (md *DbConn) SetLastCheckedForTesting(t time.Time) {
	md.lastCheckedNs.Store(t.UnixNano())
}

// ResetResolverCachesForTesting clears all resolver fallback caches under the
// shared mutex so tests start from a known state without hitting the DCS or DB.
func ResetResolverCachesForTesting() {
	resolverCacheMu.Lock()
	defer resolverCacheMu.Unlock()
	lastFoundClusterMembers = make(map[string][]PatroniClusterMember)
	lastFoundDatabases = make(map[postgresDiscoveryKey]SourceConns)
}
