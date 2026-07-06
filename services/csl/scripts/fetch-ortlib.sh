#!/usr/bin/env bash
# Prefetch the native libraries that the semantic-search (ORT) build links and
# loads at runtime. Idempotent: skips download when both libs are already in
# place. Invoked as a prerequisite of `make build`, so a clean machine builds
# without any manual setup. macOS arm64 only (the whole tool targets it).
#
#   libonnxruntime*.dylib  — microsoft/onnxruntime, loaded at runtime
#   libtokenizers.a        — daulet/tokenizers, linked at build time
#
# Override the destination with ORTLIB; pin versions with ORT_VERSION / TOK_VERSION.
set -euo pipefail

ORTLIB="${ORTLIB:-$HOME/.local/share/csl/ortlib}"
# onnxruntime_go expects ORT API 26 => onnxruntime >= 1.27 (1.20 fails to load).
ORT_VERSION="${ORT_VERSION:-1.27.0}"
TOK_VERSION="${TOK_VERSION:-1.27.0}"

ort_dylib="$ORTLIB/libonnxruntime.dylib"
tok_lib="$ORTLIB/libtokenizers.a"

if [[ -f "$ort_dylib" && -f "$tok_lib" ]]; then
  echo "ortlib: present ($ORTLIB) — skipping prefetch"
  exit 0
fi

if [[ "$(uname -s)" != "Darwin" || "$(uname -m)" != "arm64" ]]; then
  echo "ortlib: semantic search ships for macOS arm64 only (got $(uname -s)/$(uname -m))" >&2
  exit 1
fi

mkdir -p "$ORTLIB"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

ort_url="https://github.com/microsoft/onnxruntime/releases/download/v${ORT_VERSION}/onnxruntime-osx-arm64-${ORT_VERSION}.tgz"
tok_url="https://github.com/daulet/tokenizers/releases/download/v${TOK_VERSION}/libtokenizers.darwin-arm64.tar.gz"

echo "ortlib: fetching onnxruntime v${ORT_VERSION}"
curl -fsSL "$ort_url" -o "$tmp/ort.tgz"
tar xzf "$tmp/ort.tgz" -C "$tmp"
# Copy the dylibs (preserving the version symlink); skip the .dSYM debug bundle.
cp -RP "$tmp/onnxruntime-osx-arm64-${ORT_VERSION}/lib/"libonnxruntime*.dylib "$ORTLIB/"

echo "ortlib: fetching daulet/tokenizers v${TOK_VERSION}"
curl -fsSL "$tok_url" -o "$tmp/tok.tgz"
tar xzf "$tmp/tok.tgz" -C "$tmp"
cp "$tmp/libtokenizers.a" "$tok_lib"

# Ad-hoc codesign the dylib so Gatekeeper / the seatbelt sandbox loads it.
xattr -dr com.apple.provenance "$ORTLIB" 2>/dev/null || true
codesign -fs - "$ORTLIB/libonnxruntime.${ORT_VERSION}.dylib" 2>/dev/null || true

echo "ortlib: ready ($ORTLIB)"
