#!/bin/sh
# Generate a private, disposable CA and a loopback server certificate on the host.
set -eu
umask 077
dir=$(mktemp -d)
cd "$dir"
openssl req -x509 -newkey rsa:2048 -nodes -days 2 -subj /CN=atlas-test-ca \
  -addext basicConstraints=critical,CA:TRUE -keyout ca.key -out ca.crt
openssl req -new -newkey rsa:2048 -nodes -subj /CN=localhost -keyout server.key -out server.csr
printf '%s\n' 'subjectAltName=DNS:localhost,IP:127.0.0.1' 'extendedKeyUsage=serverAuth' > server.ext
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -days 2 -extfile server.ext -out server.crt
rm ca.key ca.srl server.csr server.ext
pwd
