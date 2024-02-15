# ----------------------------------------------------------------
# 1. Build Web UI
# ----------------------------------------------------------------
FROM node:19 AS uibuilder
ADD src/webui /webui
RUN cd webui && yarn install --network-timeout 100000 && yarn build

# ----------------------------------------------------------------
# 2. Build gatherer
# ----------------------------------------------------------------
FROM golang:1.22 as builder
ARG VERSION
ARG GIT_HASH
ARG GIT_TIME
ENV VERSION=${VERSION}
ENV GIT_HASH=${GIT_HASH}
ENV GIT_TIME=${GIT_TIME}

ADD src /pgwatch3
COPY --from=uibuilder /webui/build /pgwatch3/webui/build
RUN cd /pgwatch3 && go build -ldflags "-X 'main.commit=$GIT_HASH' -X 'main.date=$GIT_TIME' -X 'main.version=$VERSION'"

# ----------------------------------------------------------------
# 3. Build the final image
# ----------------------------------------------------------------
FROM alpine

# Copy over the compiled gatherer
COPY --from=builder /pgwatch3/pgwatch3 /pgwatch3/
COPY src/metrics/sql /pgwatch3/metrics

# Admin UI for configuring servers to be monitored
EXPOSE 8080
# Gatherer healthcheck port / metric statistics (JSON)
EXPOSE 8081
# Prometheus metrics scraping port
EXPOSE 9187

# Command to run the executable
ENTRYPOINT ["/pgwatch3/pgwatch3"]
