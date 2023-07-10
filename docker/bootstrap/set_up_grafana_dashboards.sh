#!/bin/bash

if [[ -f /pgwatch3/persistent-config/grafana-bootstrap-done-marker ]]; then
  exit 0
fi

declare -i attempts=$(( 60 / 5 ))
while [[ "$(curl -X GET -s -o /dev/null -w "%{http_code}" http://localhost:3000/api/health)" != "200" ]]; do
  echo "grafana server is not available"
  if (( $((attempts -= 1)) == 0 )); then
    exit 1
  fi
  sleep 5
done

timescaledb_value="false"
if [[ "${PW3_PG_SCHEMA_TYPE}" == "timescale" ]]; then
  timescaledb_value="true"
fi

# Create datasource
echo "creating datasource: pg-metrics"
echo "{
    \"orgId\": 1,
    \"name\": \"PG metrics\",
    \"uid\": \"pg-metrics\",
    \"type\": \"postgres\",
    \"access\": \"proxy\",
    \"url\": \"localhost:5432\",
    \"user\": \"pgwatch3\",
    \"database\": \"pgwatch3_metrics\",
    \"basicAuth\": false,
    \"isDefault\": true,
    \"jsonData\": {
        \"postgresVersion\": 1400,
        \"sslmode\": \"disable\",
        \"timescaledb\": ${timescaledb_value}
    },
    \"secureJsonData\": {
        \"password\": \"pgwatch3admin\"
    },
    \"version\": 0,
    \"readOnly\": false
}" | \
curl -X POST -s -w "%{http_code}" \
  -H 'Accept: application/json' \
  -H 'Content-Type: application/json' \
  -u "${PW3_GRAFANAUSER:-admin}":"${PW3_GRAFANAPASSWORD:-pgwatch3admin}" \
  -d @- \
  'http://localhost:3000/api/datasources'

# Create dashboards
GRAFANA_MAJOR_VER=$(grafana-server -v | grep -o -E '[0-9]{2}' | head -1)

for dashboard_json in $(find /pgwatch3/grafana/postgres/v"${GRAFANA_MAJOR_VER}" -name "*.json" | sort); do
  echo
  echo "creating dashboard: ${dashboard_json}"
  echo "{
      \"dashboard\": $(cat "${dashboard_json}"),
      \"message\": \"Bootstrap dashboard\",
      \"overwrite\": false,
      \"inputs\": [{
              \"name\": \"DS_PG_METRICS\",
              \"type\": \"datasource\",
              \"pluginId\": \"postgres\",
              \"value\": \"pg-metrics\"
    }]
  }" | \
  curl -X POST -s -w "%{http_code}" \
    -H 'Accept: application/json' \
    -H 'Content-Type: application/json' \
    -u "${PW3_GRAFANAUSER:-admin}":"${PW3_GRAFANAPASSWORD:-pgwatch3admin}" \
    -d @- \
    'http://localhost:3000/api/dashboards/import'
done

# Set home dashboard
# Let's make the assumption that due to the loading of dashboards in previous step in alphabetical order,
# the "Global DB overview" dashboard will always be under id=1.
# Otherwise, we can install the jq and get the identifier through the api:
#   curl -H 'Accept: application/json' 'http://localhost:3000/api/dashboards/uid/global-db-overview' | jq '.dashboard.id'
echo
echo "set global-db-overview as home page"
echo "{\"homeDashboardId\": 1}" | \
curl -X PATCH -s -w "%{http_code}" \
  -H 'Accept: application/json' \
  -H 'Content-Type: application/json' \
  -u "${PW3_GRAFANAUSER:-admin}":"${PW3_GRAFANAPASSWORD:-pgwatch3admin}" \
  -d @- \
  'http://localhost:3000/api/org/preferences'

touch /pgwatch3/persistent-config/grafana-bootstrap-done-marker

exit 0
