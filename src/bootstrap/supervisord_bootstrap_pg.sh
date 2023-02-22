#!/bin/bash

for prog in postgres grafana grafana_dashboard_setup pgwatch3 ; do
  echo "supervisorctl start $prog ..."
  supervisorctl start $prog
  echo "sleep 5"
  sleep 5
done
