#!/usr/bin/env bash
# Sign a binary with the local code signing certificate.
# Usage: sign.sh <binary-path> [bundle-identifier]
#
# Sourced by build scripts for local tools.
# Falls back to ad-hoc signing if the certificate isn't available.
set -euo pipefail

CERT_NAME="mad01 Local Signing"

mad01_sign() {
  local binary="$1"
  local identifier="${2:-}"

  local sign_args=(-f)
  if [ -n "$identifier" ]; then
    sign_args+=(-i "$identifier")
  fi

  if security find-identity -v -p codesigning 2>/dev/null | grep -q "$CERT_NAME"; then
    codesign -s "$CERT_NAME" "${sign_args[@]}" "$binary"
  else
    echo "warning: '$CERT_NAME' not found, falling back to ad-hoc signing" >&2
    codesign -s - "${sign_args[@]}" "$binary"
  fi
}

# Allow direct invocation: ./sign.sh ~/code/bin/my-tool com.mad01.my-tool
if [ "${1:-}" != "" ] && [ "${BASH_SOURCE[0]}" = "$0" ]; then
  mad01_sign "$@"
fi
