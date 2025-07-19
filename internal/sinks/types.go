package sinks

// SyncOp represents synchronization operations for metrics.
// These constants are used both in Go code and protobuf definitions.
type SyncOp int32

const (
	// InvalidOp represents an invalid or unrecognized operation
	InvalidOp SyncOp = iota // 0 
	// AddOp represents adding a new metric
	AddOp  // 1
	// DeleteOp represents deleting an existing metric or entire source
	DeleteOp // 2
	// DefineOp represents defining metric definitions
	DefineOp // 3
)

// String returns the string representation of the SyncOp
func (s SyncOp) String() string {
	switch s {
	case InvalidOp:
		return "InvalidOp"
	case AddOp:
		return "AddOp"
	case DeleteOp:
		return "DeleteOp"
	case DefineOp:
		return "DefineOp"
	default:
		return "Unknown"
	}
}
