#!/bin/bash

/pgwatch/pgwatch metric print-init full | psql -v ON_ERROR_STOP=1 --username "postgres" --dbname "pgwatch"
