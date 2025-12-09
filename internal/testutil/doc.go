// Package testutil provides testing utilities for pgwatch tests.
//
// This package contains mock implementations, test setup helpers, and constants
// used across test files. It should only be imported by test files (*_test.go)
// and will not be included in production binaries.
//
// The package includes:
//   - Mock gRPC receivers for testing sink implementations
//   - PostgreSQL container setup for integration tests
//   - gRPC server setup with TLS support for testing
//   - Test certificates and keys for TLS testing
package testutil
