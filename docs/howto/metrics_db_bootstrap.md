---
title: Bootstrapping the Metrics Measurements Database (Sink)
---

# Choosing a Database

pgwatch supports multiple databases for storing metrics measurements. The following databases are supported:

- PostgreSQL
- TimescaleDB
- CitusDB
- CockroachDB
- any other database that supports the PostgreSQL wire protocol

We will use PostgreSQL in this guide. But the steps are similar for other databases. It's up to you to choose the database that best fits your needs and set it up accordingly.

# Creating the Database

First, we need to create a database for storing the metrics measurements. We will use the `psql` command-line tool to create the database. You can also use a GUI tool like pgAdmin to create the database.

Let's assume we want to create a database named `measurements` on a completely fresh PostgreSQL installation. 
It is wise to use a special role for the metrics database, so we will create a role named `pgwatch` and assign it to the `measurements` database.

```
$ psql -U postgres -h 10.0.0.42 -p 5432 -d postgres
psql (17.2)

postgres=# CREATE ROLE pgwatch WITH LOGIN PASSWORD 'pgwatchadmin';
CREATE ROLE

postgres=# CREATE DATABASE measurements OWNER pgwatch;
CREATE DATABASE
```

That's it! We have created a database named `measurements` with the owner `pgwatch`. Now we can proceed to the next step.

# Usage

pgwatch will automatically create the necessary tables and indexes in the database when it starts. You don't need to create any tables or indexes manually.

You can now configure pgwatch to use the `measurements` database as the sink for storing metrics measurements. 

```bash
$ pgwatch --sources=/etc/sources.yaml --sink=postgresql://pgwatch@10.0.0.42/measurements
[INFO] [sink:postgresql://pgwatch@10.0.0.42/measurements] Initialising measurements database...
[INFO] [sink:postgresql://pgwatch@10.0.0.42/measurements] Measurements sink activated
...
```

That's it! You have successfully bootstrapped the metrics measurements database for pgwatch. You can now start collecting metrics from your sources and storing them in the database. 

If now you want to see the tables created by pgwatch in the `measurements` database, you can connect to the database using the `psql` command-line tool and list the tables.

```
$ psql -U pgwatch -h 10.0.0.42 -p 5432 -d measurements
psql (17.2)

measurements=> \dn
     List of schemas
     Name      |  Owner
---------------+---------
 admin         | pgwatch
 subpartitions | pgwatch
(3 rows)
```

You can see that pgwatch has created the `admin` and `subpartitions` schemas in the `measurements` database. These schemas contain the tables and indexes used by pgwatch to store metrics measurements. You may examine these schemas to understand how pgwatch stores metrics measurements in the database.

!!! tip
    You can also add `--log-level=debug` command-line parameter to see every SQL query executed by pgwatch. 
    This can be useful for debugging purposes. But remember that this will log a lot of information, 
    so it is wise to use it with empty sources this time, meaning there are no database to monitor yet.