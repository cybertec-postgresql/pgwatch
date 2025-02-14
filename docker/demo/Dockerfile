# ----------------------------------------------------------------
# 1. Build Web UI
# ----------------------------------------------------------------
FROM node:22 AS uibuilder
COPY internal/webui /webui
RUN cd webui && yarn install --network-timeout 100000 && yarn build

# ----------------------------------------------------------------
# 2. Build gatherer
# ----------------------------------------------------------------
FROM golang:1.23 AS builder

ARG VERSION
ARG GIT_HASH
ARG GIT_TIME

COPY . /pgwatch
COPY --from=uibuilder /webui/build /pgwatch/internal/webui/build
RUN cd /pgwatch && CGO_ENABLED=0 go build -ldflags "-X 'main.commit=${GIT_HASH}' -X 'main.date=${GIT_TIME}' -X 'main.version=${VERSION}'" ./cmd/pgwatch

# ----------------------------------------------------------------
# 3. Build the final image
# ----------------------------------------------------------------
FROM postgres:17-bullseye AS releaser

# Copy over the compiled gatherer
COPY --from=builder /pgwatch/pgwatch /pgwatch/
COPY internal/metrics/metrics.yaml /pgwatch/metrics/metrics.yaml

# Install TimescaleDB and extensions
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update \
    && apt-get -qy install curl gnupg postgresql-common apt-transport-https lsb-release \
    && sh /usr/share/postgresql-common/pgdg/apt.postgresql.org.sh -y \
    && curl -L "https://www.postgresql.org/media/keys/ACCC4CF8.asc" | apt-key add - \
    && curl -s https://packagecloud.io/install/repositories/timescale/timescaledb/script.deb.sh | bash \
    
    && apt-get -qy install \
        timescaledb-2-postgresql-17 postgresql-plpython3-17 postgresql-17-pg-qualstats \
        supervisor python3-psutil libfontconfig1 \
    && apt-get purge -y --auto-remove \
    && apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

# Install Grafana
RUN curl -L https://apt.grafana.com/gpg.key | apt-key add - \
    && echo "deb https://apt.grafana.com stable main" | tee /etc/apt/sources.list.d/grafana.list \
    && apt-get update \
    && apt-get -qy install grafana=10.4.15 \
    && grafana-cli plugins install marcusolsson-treemap-panel
COPY docker/demo/grafana.ini /etc/grafana/grafana.ini
COPY grafana/postgres_datasource.yml /etc/grafana/provisioning/datasources/pg_ds.yml
COPY grafana/dashboards.yml /etc/grafana/provisioning/dashboards/pg_db.yml
COPY grafana/postgres/v10/ /var/lib/grafana/dashboards/

# Set up supervisord [https://docs.docker.com/engine/admin/using_supervisord/]
COPY docker/demo/supervisord.conf /etc/supervisor/
COPY docker/bootstrap/init_persistent_config.sh \
     docker/bootstrap/init_supervisor.sh \
     docker/bootstrap/init_test_monitored_db.sh\
        /pgwatch/bootstrap/

# Set up PostgreSQL initialization scripts
RUN  mkdir -p /docker-entrypoint-initdb.d
COPY docker/bootstrap/create_role_db.sql \
     docker/bootstrap/init_dbs.sh \
        /docker-entrypoint-initdb.d/

# Apply PostgreSQL configuration
RUN  echo "include = 'pgwatch_postgresql.conf'" >> /etc/postgresql/postgresql.conf
COPY docker/demo/postgresql.conf /etc/postgresql/pgwatch_postgresql.conf

ENV PW_SOURCES=postgresql://pgwatch:pgwatchadmin@localhost:5432/pgwatch
ENV PW_SINK=postgresql://pgwatch:pgwatchadmin@localhost:5432/pgwatch_metrics
ENV POSTGRES_PASSWORD=mysecretpassword

# Admin UI for configuring servers to be monitored
EXPOSE 8080
# Postgres DB holding the pgwatch config DB / metrics
EXPOSE 5432
# Grafana UI
EXPOSE 3000
# Prometheus scraping port
EXPOSE 9187

### Volumes for easier updating to newer to newer pgwatch containers
### Backwards compatibility is not 100% guaranteed so a backup
### using traditional means is still recommended before updating - see "Updating to a newer Docker version" from README

VOLUME /pgwatch/persistent-config
VOLUME /var/lib/postgresql
VOLUME /var/lib/grafana


CMD ["/usr/bin/supervisord", "--configuration=/etc/supervisor/supervisord.conf", "--nodaemon"]