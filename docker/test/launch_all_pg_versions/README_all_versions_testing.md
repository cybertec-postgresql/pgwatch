# Docker launching scripts for metrics testing

In this folder are scripts to launch Docker containers for all supported Postgres versions (9.0-12), optionally with replicas.
By default standard Docker images are used with the following additions:

* a volume named "pg$ver" is created
* PL/Python is installed
* "psutil" Python package is installed
* "pg_stat_statements" extension is activated

# PG version to container name and port mappings

Postgres v11 container is launched under name "pg11" and exposed port will be 54311, i.e. following mapping is used:

```
for ver in {10..12}  ; do
  echo "PG v${ver} => container: pg${ver}, port: 543${ver}"
done


PG v11 => container: pg11, port: 54311
PG v12 => container: pg12, port: 54312
...
PG v15 => container: pg15, port: 54315
```

Replica port = Master port + 1000

# Speeding up testing

If there's a need to constantly launch all images with replicas, it takes quite some time for "apt update/install" so it
makes sense to do it once and then commit the changed containers into new images that can be then re-used, and adjust the
POSTGRES_IMAGE_BASE variable in both launch scripts.

```
for x in {10..12} ; do
  ver="${x}"
  pgver="${x}"
  echo "docker commit pg${ver} postgres-pw3:${pgver}"
  docker commit pg${ver} postgres-pw3:${pgver}
done
```
