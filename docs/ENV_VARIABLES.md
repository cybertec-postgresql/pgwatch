# Available env. variables by components

Some variables influence multiple components. Command line parameters override env. variables (when doing custom deployments).

## Docker image specific

- **PW3_TESTDB** When set, the config DB itself will be added to monitoring as "test". Default: -

## Gatherer daemon

See `pgwatch3 --help` output for details.

## Grafana

- **PW3_GRAFANANOANONYMOUS** Can be set to require login even for viewing dashboards. Default: -
- **PW3_GRAFANAUSER** Administrative user. Default: admin
- **PW3_GRAFANAPASSWORD** Administrative user password. Default: pgwatch3admin
- **PW3_GRAFANASSL** Use SSL. Default: -
- **PW3_GRAFANA_BASEURL** For linking to Grafana "Query details" dashboard from "Stat_stmt. overview". Default: http://0.0.0.0:3000
