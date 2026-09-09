// Package pb contains the protobuf definitions for pgwatch gRPC API.
//
// To generate the Go code from the protobuf definitions, you need to install:
//   - protoc (Protocol Buffers compiler)
//   - protoc-gen-go (Go plugin for protoc)
//   - protoc-gen-go-grpc (Go gRPC plugin for protoc)
//
// On Windows:
//
//	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12
//	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.2
//	winget install protobuf
//
// On Linux/macOS:
//
//	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12
//	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.2
//	# Install protoc via package manager (apt, brew, etc.)
//
// Then run: go generate ./api/pb/
package pb

// Generate protobuf files
//go:generate protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative pgwatch.proto
