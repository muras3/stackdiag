#!/bin/bash
set -euo pipefail

CERT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$CERT_DIR"

echo "==> Generating CA..."
openssl genrsa -out ca.key 2048 2>/dev/null
openssl req -x509 -new -nodes -key ca.key -sha256 -days 3650 \
  -subj "/CN=Bench CA" -out ca.crt

sign_cert() {
  local name=$1 cn=$2 san=$3 days=${4:-365}
  openssl genrsa -out "${name}.key" 2048 2>/dev/null
  openssl req -new -key "${name}.key" -subj "/CN=${cn}" -out "${name}.csr"
  openssl x509 -req -in "${name}.csr" -CA ca.crt -CAkey ca.key -CAcreateserial \
    -days "$days" -sha256 \
    -extfile <(printf "subjectAltName=%s" "$san") \
    -out "${name}.crt" 2>/dev/null
}

echo "==> Generating valid cert (nginx-healthy + shared services)..."
sign_cert valid nginx-healthy \
  "DNS:nginx-healthy,DNS:nginx-tls12,DNS:nginx-503,DNS:nginx-403,DNS:nginx-500,DNS:nginx-429,DNS:nginx-redirect,DNS:nginx-401"

echo "==> Generating expired cert (nginx-expired)..."
# Create with -days 1; then re-sign backdated via explicit start/end dates
openssl genrsa -out expired.key 2048 2>/dev/null
openssl req -new -key expired.key -subj "/CN=nginx-expired" -out expired.csr
# Use faketime to create an already-expired certificate
if command -v faketime >/dev/null 2>&1; then
  faketime -f '-400d' openssl x509 -req -in expired.csr \
    -CA ca.crt -CAkey ca.key -CAcreateserial \
    -days 30 -sha256 \
    -extfile <(printf "subjectAltName=DNS:nginx-expired") \
    -out expired.crt 2>/dev/null
else
  # Fallback: create cert valid for 0 days (immediately expires)
  openssl x509 -req -in expired.csr \
    -CA ca.crt -CAkey ca.key -CAcreateserial \
    -days 0 -sha256 \
    -extfile <(printf "subjectAltName=DNS:nginx-expired") \
    -out expired.crt 2>/dev/null
fi

echo "==> Generating wrong-host cert (CN=wrong.host.example.com)..."
sign_cert wronghost wrong.host.example.com "DNS:wrong.host.example.com"

echo "==> Generating expiring-soon cert (nginx-expiring, expires in 7 days)..."
sign_cert expiring nginx-expiring "DNS:nginx-expiring" 7

echo "==> Generating chain-broken cert (leaf signed by intermediate, intermediate not served)..."
# Create an intermediate CA signed by root
openssl genrsa -out intermediate.key 2048 2>/dev/null
openssl req -new -key intermediate.key -subj "/CN=Bench Intermediate CA" -out intermediate.csr
openssl x509 -req -in intermediate.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -days 365 -sha256 \
  -extfile <(printf "basicConstraints=CA:TRUE\nkeyUsage=keyCertSign,cRLSign") \
  -out intermediate.crt 2>/dev/null
# Create leaf cert signed by intermediate (NOT bundled with intermediate)
openssl genrsa -out chainbroken.key 2048 2>/dev/null
openssl req -new -key chainbroken.key -subj "/CN=nginx-chainbroken" -out chainbroken.csr
openssl x509 -req -in chainbroken.csr -CA intermediate.crt -CAkey intermediate.key -CAcreateserial \
  -days 365 -sha256 \
  -extfile <(printf "subjectAltName=DNS:nginx-chainbroken") \
  -out chainbroken.crt 2>/dev/null

echo "==> Generating self-signed cert (nginx-selfsigned, not CA-signed)..."
openssl genrsa -out selfsigned.key 2048 2>/dev/null
openssl req -x509 -new -nodes -key selfsigned.key -sha256 -days 365 \
  -subj "/CN=nginx-selfsigned" \
  -addext "subjectAltName=DNS:nginx-selfsigned" \
  -out selfsigned.crt

echo "==> Cleanup..."
rm -f *.csr *.srl

echo "==> Done:"
ls -1 *.crt *.key
