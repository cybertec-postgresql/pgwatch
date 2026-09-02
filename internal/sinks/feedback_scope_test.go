package sinks

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// feedbackAPI matches any use of the sink feedback interface.
var feedbackAPI = regexp.MustCompile(`\bFeedbacker\b|\bLastMeasurement\b|\bCanFeedback\b`)

// feedbackScopeAllowed lists the only directories permitted to mention the
// feedback API from non-test code: the package that defines it, and the
// generated gRPC stubs that carry GetLastMeasurement over the wire.
var feedbackScopeAllowed = []string{
	filepath.Join("internal", "sinks"),
	filepath.Join("api", "pb"),
}

// TestFeedbackStaysUnwired enforces AC-017: the feedback capability ships
// without a consumer. Wiring a collector to it is a separate change with a
// separate review surface (spec/design-sink-feedback.md §7.7), so a hit
// outside the allowed directories means a consumer crept in here.
func TestFeedbackStaysUnwired(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)

	var offenders []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "webui", "build":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		for _, dir := range feedbackScopeAllowed {
			if strings.HasPrefix(rel, dir+string(filepath.Separator)) {
				return nil
			}
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if feedbackAPI.Match(body) {
			offenders = append(offenders, rel)
		}
		return nil
	})
	require.NoError(t, err)

	assert.Empty(t, offenders,
		"feedback API used outside %v; adding a consumer is a separate change", feedbackScopeAllowed)
}
