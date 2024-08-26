# ----------------------------------------------------------------
# 1. Build Web UI
# ----------------------------------------------------------------
FROM node:21 AS uibuilder
ADD internal/webui /webui
RUN cd webui && yarn install --network-timeout 100000 && yarn build

# ----------------------------------------------------------------
# 2. Build gatherer
# ----------------------------------------------------------------
FROM golang:1.22 AS builder

ARG VERSION
ARG GIT_HASH
ARG GIT_TIME

COPY . /pgwatch
COPY --from=uibuilder /webui/build /pgwatch/internal/webui/build
RUN cd /pgwatch && CGO_ENABLED=0 go build -ldflags "-X 'main.commit=${GIT_HASH}' -X 'main.date=${GIT_TIME}' -X 'main.version=${VERSION}'" ./cmd/pgwatch

# ----------------------------------------------------------------
# 3. Build the final image
# ----------------------------------------------------------------
FROM alpine

# Copy over the compiled gatherer
COPY --from=builder /pgwatch/pgwatch /pgwatch/
COPY internal/metrics/metrics.yaml /pgwatch/metrics/metrics.yaml

# Admin UI for configuring servers to be monitored
EXPOSE 8080
# Gatherer healthcheck port / metric statistics (JSON)
EXPOSE 8081
# Prometheus metrics scraping port
EXPOSE 9187

# Command to run the executable
ENTRYPOINT ["/pgwatch/pgwatch"]
