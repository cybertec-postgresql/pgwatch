[![Documentation](https://img.shields.io/badge/Documentation-pgwat.ch-brightgreen)](https://pgwat.ch)
[![License: MIT](https://img.shields.io/badge/License-BSD_3-green.svg)](https://opensource.org/license/bsd-3-clause)
[![Go Build & Test](https://github.com/cybertec-postgresql/pgwatch/actions/workflows/build.yml/badge.svg)](https://github.com/cybertec-postgresql/pgwatch/actions/workflows/build.yml)
[![Coverage Status](https://coveralls.io/repos/github/cybertec-postgresql/pgwatch/badge.svg?branch=master&service=github)](https://coveralls.io/github/cybertec-postgresql/pgwatch?branch=master)


# pgwatch v3 WIP. Please do not use it in production!

This is the next generation of [pgwatch2](https://github.com/cybertec-postgresql/pgwatch2/). 

> [!WARNING]  
> This repo is under active development! Formats, schemas, and APIs are subject to rapid and backward incompatible changes!

## Quick Start

For the fastest development experience the Docker compose file is provided:

```shell
git clone https://github.com/cybertec-postgresql/pgwatch.git && cd pgwatch

docker compose -f ./docker/docker-compose.yml up --detach
```
<pre>
 ✔ Network pgwatch_default       Created
 ✔ Container pgwatch-postgres-1  Healthy
 ✔ Container pgwatch-pgwatch-1  Started
 ✔ Container pgwatch-grafana-1   Started
</pre>

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
docker compose -f ./docker/docker-compose.yml up add-test-db --force-recreate
```
<pre>
[+] Running 2/0
 ✔ Container pgwatch-postgres-1     Running                                                                       0.0s
 ✔ Container pgwatch-add-test-db-1  Created                                                                       0.0s
Attaching to pgwatch-add-test-db-1
pgwatch-add-test-db-1  | BEGIN
...
pgwatch-add-test-db-1  | GRANT
pgwatch-add-test-db-1  | COMMENT
pgwatch-add-test-db-1  | INSERT 0 1
pgwatch-add-test-db-1 exited with code 0
</pre>

## Produce Workload

To emulate workload for added test database execute:
```shell
docker compose -f ./docker/docker-compose.yml up pgbench
```
<pre>
[+] Running 2/2
 ✔ Container pgwatch-postgres-1  Running                                                                          0.0s
 ✔ Container pgwatch-pgbench-1   Created                                                                          0.1s
Attaching to pgwatch-pgbench-1
pgwatch-pgbench-1  | dropping old tables...
pgwatch-pgbench-1  | NOTICE:  table "pgbench_accounts" does not exist, skipping
pgwatch-pgbench-1  | NOTICE:  table "pgbench_branches" does not exist, skipping
pgwatch-pgbench-1  | NOTICE:  table "pgbench_history" does not exist, skipping
pgwatch-pgbench-1  | NOTICE:  table "pgbench_tellers" does not exist, skipping
pgwatch-pgbench-1  | creating tables...
pgwatch-pgbench-1  | generating data (client-side)...
pgwatch-pgbench-1  | 100000 of 5000000 tuples (2%) done (elapsed 0.11 s, remaining 5.17 s)
pgwatch-pgbench-1  | 200000 of 5000000 tuples (4%) done (elapsed 0.25 s, remaining 6.06 s)
...
pgwatch-pgbench-1  | 5000000 of 5000000 tuples (100%) done (elapsed 16.28 s, remaining 0.00 s)
pgwatch-pgbench-1  | vacuuming...
pgwatch-pgbench-1  | creating primary keys...
pgwatch-pgbench-1  | done in 42.29 s (drop tables 0.03 s, create tables 0.04 s, client-side generate 18.23 s, vacuum 1.29 s, primary keys 22.70 s).
pgwatch-pgbench-1  | pgbench (15.4)
pgwatch-pgbench-1  | starting vacuum...
pgwatch-pgbench-1  | end.
pgwatch-pgbench-1  | progress: 5.0 s, 642.2 tps, lat 15.407 ms stddev 11.794, 0 failed
pgwatch-pgbench-1  | progress: 10.0 s, 509.6 tps, lat 19.541 ms stddev 9.493, 0 failed
...
pgwatch-pgbench-1  | progress: 185.0 s, 325.3 tps, lat 16.825 ms stddev 8.330, 0 failed
pgwatch-pgbench-1  |
pgwatch-pgbench-1  |
pgwatch-pgbench-1  | transaction type: builtin: TPC-B (sort of)
pgwatch-pgbench-1  | scaling factor: 50
pgwatch-pgbench-1  | query mode: simple
pgwatch-pgbench-1  | number of clients: 10
pgwatch-pgbench-1  | number of threads: 2
pgwatch-pgbench-1  | maximum number of tries: 1
pgwatch-pgbench-1  | number of transactions per client: 10000
pgwatch-pgbench-1  | number of transactions actually processed: 100000/100000
pgwatch-pgbench-1  | number of failed transactions: 0 (0.000%)
pgwatch-pgbench-1  | latency average = 18.152 ms
pgwatch-pgbench-1  | latency stddev = 13.732 ms
pgwatch-pgbench-1  | initial connection time = 25.085 ms
pgwatch-pgbench-1  | tps = 534.261013 (without initial connection time)
pgwatch-pgbench-1  | dropping old tables...
pgwatch-pgbench-1  | done in 0.45 s (drop tables 0.45 s).
pgwatch-pgbench-1 exited with code 0
</pre>

## Inspect database

> [!IMPORTANT]
pgAdmin uses port 80. If you want it to use another port, change it in `docker-compose.yml` file.

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
