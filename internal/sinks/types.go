package sinks

// SyncOp represents synchronization operations for metrics.
// These constants are used both in Go code and protobuf definitions.
type SyncOp int32

const (
	// InvalidOp represents an invalid or unrecognized operation
	InvalidOp = -1
	// AddOp represents adding a new metric
	AddOp SyncOp = iota // 0
	// DeleteOp represents deleting an existing metric
	DeleteOp // 1
	// DefineOp represents defining metric definitions
	DefineOp // 2
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
