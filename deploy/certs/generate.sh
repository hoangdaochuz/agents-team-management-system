#!/usr/bin/env bash
# Generates a local CA + mTLS client/server certificates for the Settings <-> Runner
# internal decrypt channel. DEV-ONLY. Do not use these in production.
#
# Produces:
#   ca.crt, ca.key                  (internal CA)
#   settings.crt, settings.key      (Settings server identity)
#   runner.crt, runner.key          (Runner client identity)
#
# Run from anywhere; outputs land in this script's directory.
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$DIR"

# Reuse the same SAN on both certs: services reach each other by docker-compose
# service name (settings, runner) and by localhost for dev.
SAN="DNS:settings,DNS:runner,DNS:localhost,IP:127.0.0.1"

echo ">> CA"
[ -f ca.key ] || openssl genrsa -out ca.key 4096 2>/dev/null
[ -f ca.crt ] || openssl req -x509 -new -nodes -key ca.key -sha256 -days 3650 \
  -subj "/CN=aaks-internal-dev-ca" -out ca.crt 2>/dev/null

mk_cert () {
  local name="$1"
  echo ">> $name"
  [ -f "$name.key" ] || openssl genrsa -out "$name.key" 4096 2>/dev/null
  [ -f "$name.csr" ] || openssl req -new -key "$name.key" \
    -subj "/CN=$name" -out "$name.csr" 2>/dev/null
  local ext; ext="$(mktemp)"
  cat > "$ext" <<EOF
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth, clientAuth
subjectAltName = $SAN
EOF
  [ -f "$name.crt" ] || openssl x509 -req -in "$name.csr" -CA ca.crt -CAkey ca.key \
    -CAcreateserial -out "$name.crt" -days 825 -sha256 -extfile "$ext" 2>/dev/null
  rm -f "$ext"
}

mk_cert settings
mk_cert runner

echo ">> done. Dev certs in $DIR"
echo "   (ca.crt, settings.{crt,key}, runner.{crt,key})"
