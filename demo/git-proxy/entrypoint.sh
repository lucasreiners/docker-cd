#!/bin/sh
set -eu
if [ ! -s /certs/git.crt ]; then
  openssl req -x509 -newkey rsa:2048 -nodes -days 365 \
    -keyout /certs/git.key -out /certs/git.crt \
    -subj '/CN=git-https' -addext 'subjectAltName=DNS:git-https,DNS:localhost' \
    -addext 'basicConstraints=critical,CA:TRUE'
  chmod 600 /certs/git.key
  chmod 644 /certs/git.crt
fi
exec nginx -g 'daemon off;'
