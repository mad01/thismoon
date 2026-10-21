#!/usr/bin/env bash
set -euo pipefail

CERT_NAME="mad01 Local Signing"

# Idempotent: skip if cert already exists and is trusted for codesigning
if security find-identity -v -p codesigning 2>/dev/null | grep -q "$CERT_NAME"; then
  echo "codesign: certificate '$CERT_NAME' already exists and is trusted"
  exit 0
fi

echo "codesign: creating self-signed code signing certificate..."

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

cat > "$WORK/cert.cfg" << 'EOF'
[ req ]
default_bits       = 2048
distinguished_name = req_dn
prompt             = no
[ req_dn ]
CN = mad01 Local Signing
EOF

openssl req -x509 -newkey rsa:2048 \
  -keyout "$WORK/signing.key" \
  -out "$WORK/signing.crt" \
  -days 3650 -nodes \
  -config "$WORK/cert.cfg" \
  -addext "keyUsage=digitalSignature" \
  -addext "extendedKeyUsage=codeSigning" \
  2>/dev/null

# -legacy needed for macOS Keychain compatibility with modern OpenSSL
openssl pkcs12 -export \
  -out "$WORK/signing.p12" \
  -inkey "$WORK/signing.key" \
  -in "$WORK/signing.crt" \
  -passout pass:setup \
  -legacy 2>/dev/null

security import "$WORK/signing.p12" \
  -k ~/Library/Keychains/login.keychain-db \
  -P setup \
  -T /usr/bin/codesign

echo "codesign: trusting certificate for code signing (may prompt for password)..."
security add-trusted-cert -d -r trustRoot -p codeSign \
  -k ~/Library/Keychains/login.keychain-db \
  "$WORK/signing.crt"

if security find-identity -v -p codesigning 2>/dev/null | grep -q "$CERT_NAME"; then
  echo "codesign: ✓ certificate '$CERT_NAME' ready"
else
  echo "codesign: ✗ certificate not found after import; check Keychain Access" >&2
  exit 1
fi
