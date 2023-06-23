#!/bin/bash

mkdir /var/run/grafana && chown grafana /var/run/grafana

if [ ! -f /pgwatch3/persistent-config/self-signed-ssl.key -o ! -f /pgwatch3/persistent-config/self-signed-ssl.pem ] ; then
    openssl req -x509 -newkey rsa:4096 -keyout /pgwatch3/persistent-config/self-signed-ssl.key -out /pgwatch3/persistent-config/self-signed-ssl.pem -days 3650 -nodes -sha256 -subj '/CN=pw3'
    cp /pgwatch3/persistent-config/self-signed-ssl.pem /etc/ssl/certs/ssl-cert-snakeoil.pem
    cp /pgwatch3/persistent-config/self-signed-ssl.key /etc/ssl/private/ssl-cert-snakeoil.key
    chown postgres /etc/ssl/certs/ssl-cert-snakeoil.pem /etc/ssl/private/ssl-cert-snakeoil.key
    chmod -R 0600 /etc/ssl/certs/ssl-cert-snakeoil.pem /etc/ssl/private/ssl-cert-snakeoil.key
    chmod -R o+rx /pgwatch3/persistent-config
fi

# enable password encryption by default from v1.8.0
if [ ! -f /pgwatch3/persistent-config/default-password-encryption-key.txt ]; then
  echo -n "${RANDOM}${RANDOM}${RANDOM}${RANDOM}" > /pgwatch3/persistent-config/default-password-encryption-key.txt
  chown postgres /pgwatch3/persistent-config/default-password-encryption-key.txt
  chmod 0600 /pgwatch3/persistent-config/default-password-encryption-key.txt
fi

GRAFANASSL="${PW3_GRAFANASSL,,}"    # to lowercase
if [ "$GRAFANASSL" == "1" ] || [ "${GRAFANASSL:0:1}" == "t" ]; then
    $(grep -q 'protocol = http$' /etc/grafana/grafana.ini)
    if [ "$?" -eq 0 ] ; then
        sed -i 's/protocol = http.*/protocol = https/' /etc/grafana/grafana.ini
    fi
fi

if [ -n "$PW3_GRAFANAUSER" ] ; then
    sed -i "s/admin_user =.*/admin_user = ${PW3_GRAFANAUSER}/" /etc/grafana/grafana.ini
fi

if [ -n "$PW3_GRAFANAPASSWORD" ] ; then
    sed -i "s/admin_password =.*/admin_password = ${PW3_GRAFANAPASSWORD}/" /etc/grafana/grafana.ini
fi

if [ -n "$PW3_GRAFANANOANONYMOUS" ] ; then
CFG=$(cat <<-'HERE'
[auth.anonymous]
enabled = false
HERE
)
echo "$CFG" >> /etc/grafana/grafana.ini
fi