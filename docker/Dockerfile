# ================================================================
# PRODUCTION PGWATCH IMAGE - OPTIMIZED VERSION
# Uses shared base stages for faster builds and better caching
# ================================================================

# ----------------------------------------------------------------
# 1. WebUI Dependencies (cached separately for better performance)
# ----------------------------------------------------------------
FROM node:22 AS webui-deps
WORKDIR /webui
# Copy only package files first for optimal layer caching
COPY internal/webui/package.json internal/webui/yarn.lock ./
RUN yarn install --network-timeout 100000 --frozen-lockfile

# ----------------------------------------------------------------
# 2. WebUI Builder (rebuilds only when source changes)
# ----------------------------------------------------------------
FROM webui-deps AS webui-builder
# Copy source files and build
COPY internal/webui/ ./
RUN yarn build

# ----------------------------------------------------------------
# 3. Go Dependencies and Tools (cached separately)
# ----------------------------------------------------------------
FROM golang:1.25 AS go-deps
# Install protoc and required tools for protobuf generation
RUN apt-get update && apt-get install -y protobuf-compiler && rm -rf /var/lib/apt/lists/*
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
RUN go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Pre-download Go modules for better caching
WORKDIR /pgwatch
COPY go.mod go.sum ./
RUN go mod download

# ----------------------------------------------------------------
# 4. Go Builder (application compilation)
# ----------------------------------------------------------------
FROM go-deps AS go-builder

ARG VERSION
ARG GIT_HASH
ARG GIT_TIME

# Copy source code
COPY . .
# Copy built WebUI from previous stage
COPY --from=webui-builder /webui/build ./internal/webui/build

# Generate protobuf and build the application
RUN go generate ./api/pb/ && \
    CGO_ENABLED=0 GOEXPERIMENT=greenteagc go build -ldflags "\
        -X 'main.commit=${GIT_HASH}' \
        -X 'main.date=${GIT_TIME}' \
        -X 'main.version=${VERSION}'" ./cmd/pgwatch

# ----------------------------------------------------------------
# 5. Final production image
# ----------------------------------------------------------------
FROM alpine

# Copy over the compiled gatherer
COPY --from=go-builder /pgwatch/pgwatch /pgwatch/
COPY internal/metrics/metrics.yaml /pgwatch/metrics/metrics.yaml

# Admin UI for configuring servers to be monitored
EXPOSE 8080

# Command to run the executable
ENTRYPOINT ["/pgwatch/pgwatch"]
