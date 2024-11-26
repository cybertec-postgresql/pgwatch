[![Documentation](https://img.shields.io/badge/Documentation-pgwat.ch-brightgreen)](https://pgwat.ch)
[![License: MIT](https://img.shields.io/badge/License-BSD_3-green.svg)](https://opensource.org/license/bsd-3-clause)
[![Go Build & Test](https://github.com/cybertec-postgresql/pgwatch/actions/workflows/build.yml/badge.svg)](https://github.com/cybertec-postgresql/pgwatch/actions/workflows/build.yml)
[![Coverage Status](https://coveralls.io/repos/github/cybertec-postgresql/pgwatch/badge.svg?branch=master&service=github)](https://coveralls.io/github/cybertec-postgresql/pgwatch?branch=master)


# pgwatch v3-beta. Please test it as much as possible!

This is the next generation of [pgwatch2](https://github.com/cybertec-postgresql/pgwatch2/). 

## Quick Start

To fetch and run the latest **demo** Docker image, exposing 
- Grafana on port 3000, 
- the administrative web UI on port 8080,
- the internal configuration and metrics database on port 5432:

```shell
docker run -d --name pw3 -p 5432:5432 -p 3000:3000 -p 8080:8080 -e PW_TESTDB=true cybertecpostgresql/pgwatch-demo
```

After some minutes you could open the ["Database Overview"](http://127.0.0.1:3000/d/db-overview/db-overview) dashboard and start looking at metrics. For defining your own dashboards you need to log in Grafana as admin (`admin`/`pgwatchadmin`).

If you don't want to add the test database for monitoring, remove the `PW_TESTDB` parameter when launching the container.



## Development and production use

For production and long term installation `cybertecpostgresql/pgwatch` Docker image should be used. 
For the fastest development and deployment experience the Docker compose files are provided:

```shell
git clone https://github.com/cybertec-postgresql/pgwatch.git && cd pgwatch

docker compose -f ./docker/docker-compose.yml up --detach
```
```console
 ✔ Network pgwatch_default       Created
 ✔ Container pgwatch-postgres-1  Healthy
 ✔ Container pgwatch-pgwatch-1   Started
 ✔ Container pgwatch-grafana-1   Started
```

These commands will build and start services listed in the compose file:
- configuration and metric database;
- pgwatch monitoring agent with WebUI;
- Grafana with dashboards.

## Monitor Database

After start, you could open the [monitoring dashboard](http://localhost:3000/) and start
looking at metrics.

To add a test database under monitoring, you can use [built-in WebUI](http://localhost:8080/). Or simply
execute from command line:
```shell
docker/compose.add-test-db.sh
```

```console
CREATE EXTENSION
CREATE EXTENSION
CREATE FUNCTION
GRANT
GRANT
GRANT
INSERT 0 1
```

## Produce Workload

To emulate workload for added test database execute:
```shell
docker/compose.pgbench.sh 
```
```console
dropping old tables...
creating tables...
generating data (client-side)...
vacuuming...
creating primary keys...
done in 16.82 s (drop tables 0.14 s, create tables 0.01 s, client-side generate 14.18 s, vacuum 0.21 s, primary keys 2.28 s).
pgbench (17.2 (Debian 17.2-1.pgdg120+1))
starting vacuum...end.
progress: 5.0 s, 1888.9 tps, lat 5.224 ms stddev 3.512, 0 failed
^Csh-5.2$ docker/compose.pgbench.sh
dropping old tables...
creating tables...
generating data (client-side)...
vacuuming...
creating primary keys...
done in 9.46 s (drop tables 0.11 s, create tables 0.01 s, client-side generate 6.95 s, vacuum 0.26 s, primary keys 2.13 s).   
pgbench (17.2 (Debian 17.2-1.pgdg120+1))
starting vacuum...end.
progress: 5.0 s, 1551.3 tps, lat 3.805 ms stddev 11.169, 0 failed
progress: 10.0 s, 307.5 tps, lat 45.560 ms stddev 380.998, 0 failed
progress: 15.0 s, 2579.0 tps, lat 3.865 ms stddev 2.611, 0 failed
progress: 20.0 s, 1974.9 tps, lat 5.038 ms stddev 4.808, 0 failed
progress: 25.0 s, 1414.9 tps, lat 7.048 ms stddev 5.124, 0 failed
progress: 30.0 s, 1643.0 tps, lat 6.056 ms stddev 4.395, 0 failed
progress: 35.0 s, 947.4 tps, lat 10.423 ms stddev 34.786, 0 failed
progress: 40.0 s, 1832.3 tps, lat 5.485 ms stddev 4.438, 0 failed
progress: 45.0 s, 1541.0 tps, lat 6.456 ms stddev 4.135, 0 failed
progress: 50.0 s, 2017.3 tps, lat 4.938 ms stddev 3.316, 0 failed
progress: 55.0 s, 1730.3 tps, lat 5.751 ms stddev 4.706, 0 failed
progress: 60.0 s, 1363.6 tps, lat 7.302 ms stddev 32.543, 0 failed
transaction type: <builtin: TPC-B (sort of)>
scaling factor: 50
query mode: simple
number of clients: 10
number of threads: 2
maximum number of tries: 1
number of transactions per client: 10000
number of transactions actually processed: 100000/100000
number of failed transactions: 0 (0.000%)
latency average = 6.253 ms
latency stddev = 49.094 ms
initial connection time = 15.263 ms
tps = 1567.619330 (without initial connection time)
dropping old tables...
done in 0.11 s (drop tables 0.11 s).
```


## Inspect database

> [!IMPORTANT]
pgAdmin uses port 80. If you want it to use another port, change it in `docker/compose.pgadmin.yml` file.

To look what is inside `pgwatch` database, you can spin up pgAdmin4:
```shell
docker compose -f ./docker/docker-compose.yml up --detach pgadmin
```
Go to `localhost` in your favorite browser and login as `admin@local.com`, password `admin`.
Server `pgwatch` should be already added in `Servers` group.

## Development

If you apply any changes to the source code and want to restart the agent, it's usually enough to run:

```shell
docker compose -f ./docker/docker-compose.yml up pgwatch --build --force-recreate --detach
```

The command above will rebuild the `pgwatch` agent from sources and relaunch the container.

## Logs

If you are running containers in detached mode, you still can follow the logs:
```shell
docker compose -f ./docker/docker-compose.yml logs --follow
```

Or you may check the log of a particular service:
```shell
docker compose -f ./docker/docker-compose.yml logs pgwatch --follow
```

# Contributing

Feedback, suggestions, problem reports, and pull requests are very much appreciated.
