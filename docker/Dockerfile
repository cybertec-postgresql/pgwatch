# ----------------------------------------------------------------
# 1. Build Web UI
# ----------------------------------------------------------------
FROM node:22 AS uibuilder
ADD internal/webui /webui
RUN cd webui && yarn install --network-timeout 100000 && yarn build

# ----------------------------------------------------------------
# 2. Build gatherer
# ----------------------------------------------------------------
FROM golang:1.24 AS builder

ARG VERSION
ARG GIT_HASH
ARG GIT_TIME

# Install protoc and required tools for protobuf generation
RUN apt-get update && apt-get install -y protobuf-compiler && rm -rf /var/lib/apt/lists/*
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
RUN go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

COPY . /pgwatch
COPY --from=uibuilder /webui/build /pgwatch/internal/webui/build
RUN cd /pgwatch \
    && go generate ./api/pb/ \
    && CGO_ENABLED=0 go build -ldflags "\
        -X 'main.commit=${GIT_HASH}' \
        -X 'main.date=${GIT_TIME}' \
        -X 'main.version=${VERSION}'" ./cmd/pgwatch

# ----------------------------------------------------------------
# 3. Build the final image
# ----------------------------------------------------------------
FROM alpine

# Copy over the compiled gatherer
COPY --from=builder /pgwatch/pgwatch /pgwatch/
COPY internal/metrics/metrics.yaml /pgwatch/metrics/metrics.yaml

# Admin UI for configuring servers to be monitored
EXPOSE 8080

# Command to run the executable
ENTRYPOINT ["/pgwatch/pgwatch"]
