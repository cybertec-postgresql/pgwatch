#!/usr/bin/env bash

###
### For building the pgwatch3-webui docker image
###

# currently only checks if SSL is enabled and if so generates new cert on the first run
if [ ! -f /pgwatch3/persistent-config/self-signed-ssl.key -o ! -f /pgwatch3/persistent-config/self-signed-ssl.pem ] ; then
    openssl req -x509 -newkey rsa:4096 -keyout /pgwatch3/persistent-config/self-signed-ssl.key -out /pgwatch3/persistent-config/self-signed-ssl.pem -days 3650 -nodes -sha256 -subj '/CN=pw3'
    chmod 0600 /pgwatch3/persistent-config/self-signed-ssl.*
fi

exec /pgwatch3/webpy/web.py "$@"
