# Available env. variables by components

Some variables influence multiple components. Command line parameters override env. variables (when doing custom deployments).

## Docker image specific

- **PW_TESTDB** When set, the config DB itself will be added to monitoring as "test". Default: -

## Gatherer daemon

See `pgwatch --help` output for details.

## Grafana

- **PW_GRAFANANOANONYMOUS** Can be set to require login even for viewing dashboards. Default: -
- **PW_GRAFANAUSER** Administrative user. Default: admin
- **PW_GRAFANAPASSWORD** Administrative user password. Default: pgwatchadmin
- **PW_GRAFANASSL** Use SSL. Default: -
- **PW_GRAFANA_BASEURL** For linking to Grafana "Query details" dashboard from "Stat_stmt. overview". Default: http://0.0.0.0:3000
