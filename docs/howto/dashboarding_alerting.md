---
title: Dashboarding and alerting
---

# Grafana intro

To display the gathered and stored metrics the pgwatch project has
decided to rely heavily on the popular Grafana dashboarding solution.
This means only though that it's installed in the default Docker images
and there's a set of predefined dashboards available to cover most of
the metrics gathered via the *Preset Configs*.

This does not mean though that Grafana is in any way tightly coupled
with project's other components - quite the opposite actually, one can
use any other means / tools to use the metrics data gathered by the
pgwatch daemon.

Currently, there are around 30 preset dashboards available for PostgreSQL
data sources. Due to that nowadays, if metric gathering volumes are not
a problem, we recommend using Postgres storage for most users.

Note though that most users will probably want to always adjust the
built-in dashboards slightly (colors, roundings, etc.), so that they
should be taken only as examples to quickly get started. Also note that
in case of changes it's not recommended to change the built-in ones,
but use the *Save as* features - this will allow later to easily update
all the dashboards *en masse* per script, without losing any custom user
changes.

**Links:**

[Built-in dashboards for PostgreSQL (TimescaleDB)
storage](https://github.com/cybertec-postgresql/pgwatch/tree/master/grafana/postgres/)

[Screenshots of pgwatch default
dashboards](https://github.com/cybertec-postgresql/pgwatch/tree/master/docs/screenshots)

[The online Demo site](https://demo.pgwatch.com/)

# Alerting

Alerting is very conveniently also supported by Grafana in a simple
point-and-click style - see
[here](https://grafana.com/docs/grafana/latest/alerting/alerts-overview/)
for the official documentation. In general all more popular notification
services are supported, and it's pretty much the easiest way to quickly
start with PostgreSQL alerting on a smaller scale. For enterprise usage
with hundreds of instances it's might get too "clicky" though and
there are also some limitations - currently you can set alerts only on
Graph panels and there must be no variables used in the query so you
cannot use most of the pre-created pgwatch graphs, but need to create
your own.

Nevertheless, alerting via Grafana is s a good option for lighter use
cases and there's also a preset dashboard template named "Alert
Template" from the pgwatch project to give you some ideas on what to
alert on.

Note though that alerting is always a bit of a complex topic - it
requires good understanding of PostgreSQL operational metrics and also
business criticality background infos, so we don't want to be too
opinionated here, and it's up to the users to implement.
