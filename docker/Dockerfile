# ----------------------------------------------------------------
# 1. Build Web UI
# ----------------------------------------------------------------
FROM node:19 AS uibuilder
ADD src/webui /webui
RUN cd webui && yarn install --network-timeout 100000 && yarn build

# ----------------------------------------------------------------
# 2. Build gatherer
# ----------------------------------------------------------------
FROM golang:1.20 as builder
ARG GIT_HASH
ARG GIT_TIME
ENV GIT_HASH=${GIT_HASH}
ENV GIT_TIME=${GIT_TIME}

ADD src /pgwatch3
COPY --from=uibuilder /webui/build /pgwatch3/webui/build
RUN cd /pgwatch3 && bash build_gatherer.sh

# ----------------------------------------------------------------
# 3. Build the final image
# ----------------------------------------------------------------
FROM postgres:15-bullseye AS releaser

# Copy over the compiled gatherer
COPY --from=builder /pgwatch3/pgwatch3 /pgwatch3/
COPY src/metrics /pgwatch3/

# Install dependancies
RUN apt-get -q update \
    && DEBIAN_FRONTEND=noninteractive apt-get -qy install \
        wget postgresql-plpython3-15 postgresql-15-pg-qualstats supervisor python3-psutil libfontconfig1 \
    && apt-get purge -y --auto-remove \
    && apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

# Install Grafana
RUN arch=$(arch | sed s/aarch64/arm64/ | sed s/x86_64/amd64/) \
    && wget -q -O grafana.deb "https://dl.grafana.com/oss/release/grafana_9.5.3_${arch}.deb" \
    && dpkg -i grafana.deb && rm grafana.deb
ADD  grafana_dashboards /pgwatch3/grafana_dashboards
COPY grafana_dashboards/grafana_custom_config.ini /etc/grafana/grafana.ini

# Set up supervisord [https://docs.docker.com/engine/admin/using_supervisord/]
COPY docker/supervisord-postgres.conf 
        /etc/supervisor/supervisord.conf
COPY src/bootstrap/init_persistent_config.sh \
     src/bootstrap/set_up_grafana_dashboards.sh \
     src/bootstrap/init_supervisor.sh \
        /pgwatch3/bootstrap/

# Set up PostgreSQL initialization scripts
RUN  mkdir -p /docker-entrypoint-initdb.d
COPY src/bootstrap/1_create_role_db.sql \
     src/bootstrap/2_init_pgwatch_db.sh \
        /docker-entrypoint-initdb.d/

# Apply PostgreSQL configuration
RUN  echo "include = 'pgwatch_postgresql.conf'" >> /etc/postgresql/postgresql.conf
COPY docker/postgresql.conf /etc/postgresql/pgwatch_postgresql.conf

ENV PW3_DATASTORE postgres
ENV PW3_PG_METRIC_STORE_CONN_STR postgresql://pgwatch3:pgwatch3admin@localhost:5432/pgwatch3_metrics
ENV PW3_PG_SCHEMA_TYPE metric-time
ENV PW3_AES_GCM_KEYPHRASE_FILE /pgwatch3/persistent-config/default-password-encryption-key.txt
ENV POSTGRES_PASSWORD=mysecretpassword

# Admin UI for configuring servers to be monitored
EXPOSE 8080
# Gatherer healthcheck port / metric statistics (JSON)
EXPOSE 8081
# Postgres DB holding the pgwatch3 config DB / metrics
EXPOSE 5432
# Grafana UI
EXPOSE 3000
# Prometheus scraping port
EXPOSE 9187

### Volumes for easier updating to newer to newer pgwatch3 containers
### Backwards compatibility is not 100% guaranteed so a backup
### using traditional means is still recommended before updating - see "Updating to a newer Docker version" from README

VOLUME /pgwatch3/persistent-config
VOLUME /var/lib/postgresql
VOLUME /var/lib/grafana


CMD ["/usr/bin/supervisord", "--configuration=/etc/supervisor/supervisord.conf", "--nodaemon"]