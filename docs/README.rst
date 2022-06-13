Introduction
============


pgwatch3 is a flexible PostgreSQL-specific monitoring solution, relying on Grafana dashboards for the UI part. It supports monitoring
of almost all metrics for Postgres versions 9.0 to 13 out of the box and can be easily extended to include custom metrics.
At the core of the solution is the metrics gathering daemon written in Go, with many options to configure the details and
aggressiveness of monitoring, types of metrics storage and the display the metrics.

Quick start with Docker
-----------------------

For the fastest setup experience Docker images are provided via Docker Hub (if new to Docker start `here <https://docs.docker.com/get-started/>`_).
For custom setups see the :ref:`Custom installations <custom_installation>` paragraph below or turn to the pre-built DEB / RPM / Tar
packages on the Github Releases `page <https://github.com/cybertec-postgresql/pgwatch3/releases>`_.

Launching the latest pgwatch3 Docker image with built-in Postgres metrics storage DB:

::

    # run the latest Docker image, exposing Grafana on port 3000 and the administrative web UI on 8080
    docker run -d -p 3000:3000 -p 8080:8080 -e PW3_TESTDB=true --name pw3 cybertec/pgwatch3

After some minutes you could for example open the `"DB overview" <http://127.0.0.1:3000/dashboard/db/db-overview>`_ dashboard and start
looking at metrics in Grafana. For defining your own dashboards or making changes you need to log in as admin (default
user/password: admin/pgwatch3admin).

NB! If you don't want to add the "test" database (the pgwatch3 configuration DB holding connection strings to monitored DBs
and metric definitions) to monitoring, remove the PW3_TESTDB env variable.

Also note that for long term production usage with Docker it's highly recommended to use separate *volumes* for each
pgwatch3 component - see :ref:`here <docker_example_launch>` for a better launch example.

.. _typical_architecture:

Typical "pull" architecture
---------------------------

To get an idea how pgwatch3 is typically deployed a diagram of the standard Docker image fetching metrics from a set of
Postgres databases configured via a configuration DB:

.. image:: https://raw.githubusercontent.com/cybertec-postgresql/pgwatch3/master/docs/screenshots/pgwatch3_architecture.png
   :alt: pgwatch3 typical deployment architecture diagram
   :target: https://raw.githubusercontent.com/cybertec-postgresql/pgwatch3/master/docs/screenshots/pgwatch3_architecture.png

Typical "push" architecture
---------------------------

A better fit for very dynamic (Cloud) environments might be a more de-centralized "push" approach or just exposing the metrics
over a port for remote scraping. In that case the only component required would be the pgwatch3 metrics collection daemon.

.. image:: https://raw.githubusercontent.com/cybertec-postgresql/pgwatch3/master/docs/screenshots/pgwatch3_architecture_push.png
   :alt: pgwatch3 "push" deployment architecture diagram
   :target: https://raw.githubusercontent.com/cybertec-postgresql/pgwatch3/master/docs/screenshots/pgwatch3_architecture_push.png
