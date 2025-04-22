#!/bin/bash
# Only allow replication user to connect for replication purposes

echo "host    replication     postgres    0.0.0.0/0    trust" >> "$PGDATA/pg_hba.conf"
