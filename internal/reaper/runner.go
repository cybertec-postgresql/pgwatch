package reaper

import "context"

// SourceRunner is the interface for components that run metric collection for a single source.
type SourceRunner interface {
	Run(ctx context.Context)
}

var _ SourceRunner = (*SourceReaper)(nil)
