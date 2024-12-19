#!/bin/bash

/pgwatch/bootstrap/init_persistent_config.sh

supervisorctl start postgres
sleep 10
until pg_isready ; do sleep 10 ; done

for prog in grafana pgwatch ; do
  echo "supervisorctl start $prog ..."'1' 
  supervisorctl start $prog
  echo "sleep 5"
  sleep 5
done

/pgwatch/bootstrap/init_test_monitored_db.sh
