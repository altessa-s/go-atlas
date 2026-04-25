#!/bin/bash

# Generate CA key and certificate
openssl genrsa -out ca.key 2048
openssl req -new -x509 -days 365 -key ca.key -out ca.crt -subj "/C=US/ST=Test/L=Test/O=Test CA/CN=Test CA"

# Generate server key and certificate
openssl genrsa -out server.key 2048
openssl req -new -key server.key -out server.csr -subj "/C=US/ST=Test/L=Test/O=Test/CN=localhost"
openssl x509 -req -days 365 -in server.csr -CA ca.crt -CAkey ca.key -set_serial 1 -out server.crt
rm server.csr

# Generate encrypted server key in legacy format (with password "testpass")
openssl rsa -aes256 -in server.key -out server_encrypted_legacy.key -passout pass:testpass -traditional

# Generate PKCS#8 unencrypted key
openssl pkcs8 -topk8 -nocrypt -in server.key -out server_pkcs8.key

# Generate PKCS#8 encrypted key (with password "testpass")
openssl pkcs8 -topk8 -in server.key -out server_pkcs8_encrypted.key -passout pass:testpass

# Generate EC key and certificate
openssl ecparam -genkey -name prime256v1 -out ec.key
openssl req -new -x509 -key ec.key -out ec.crt -days 365 -subj "/C=US/ST=Test/L=Test/O=Test/CN=ec.test"

# Generate concatenated cert+key file
cat server.crt server.key > server_combined.pem

# Generate invalid certificate (expired)
# Generate a certificate that expires immediately
openssl req -new -key server.key -out expired.csr -subj "/C=US/ST=Test/L=Test/O=Test/CN=expired"
openssl x509 -req -days 1 -in expired.csr -CA ca.crt -CAkey ca.key -set_serial 2 -out expired.crt
rm expired.csr

# Generate invalid PEM file
echo "This is not a valid certificate" > invalid.pem

echo "Test certificates generated successfully"
