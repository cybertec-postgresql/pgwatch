#!/bin/bash

# Normally we insert the local pwatch2 config DB as "test" database for monitoring to instantly get some sample graphs

export PGUSER=postgres

if [ -z "$NOTESTDB" ] ; then

if [ ! -f /pgwatch3/test_db_installed_marker ] ; then

while true ; do

  # It will take some time for Postgres to start
  sleep 1

  $(pg_isready -q)

  if [[ "$?" -ne 0 ]] ; then
    continue
  else
    break
  fi

done

SQL=$(cat <<-HERE
insert into pgwatch3.monitored_db (md_unique_name, md_preset_config_name, md_config, md_hostname, md_port, md_dbname, md_user, md_password)
  select 'test', 'full', null, 'localhost', '5432', 'pgwatch3', 'pgwatch3', 'pgwatch3admin'
  where not exists (
    select * from pgwatch3.monitored_db where (md_unique_name, md_hostname, md_dbname) = ('test', 'localhost', 'pgwatch3')
  )
HERE
)

psql -c "$SQL" pgwatch3

if [ $? -eq 0 ] ; then
 touch /pgwatch3/test_db_installed_marker
fi

fi

fi
