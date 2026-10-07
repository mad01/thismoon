#!/bin/bash
# Fetch the page shell's vendored assets (browser libraries and fonts) into
# the workdir at install time; they are not committed (docs/adr/0022).
# /assets/ is served immutable for a year, so every filename carries its
# version. A file upstream ships as-is also carries a pinned sha256.
set -euo pipefail

WORKDIR="${PRESENT_WORKDIR:-$HOME/.config/present}"
ASSETS="$WORKDIR/assets"
CYTOSCAPE_VERSION="3.31.0"
DAGRE_VERSION="0.8.5"
CY_DAGRE_VERSION="2.5.0"
ELK_VERSION="0.12.0"
CY_ELK_VERSION="2.3.0"
CHART_VERSION="4.4.6"
SANKEY_VERSION="0.15.3"
D3_VERSION="7.9.0"
D3_SHA256="f2094bbf6141b359722c4fe454eb6c4b0f0e42cc10cc7af921fc158fceb86539"
FONTS_URL="https://fonts.googleapis.com/css2?family=Fira+Code:wght@300..700&family=Inter:ital,opsz,wght@0,14..32,300..700;1,14..32,300..700&family=Lexend:wght@300..700&family=Work+Sans:ital,wght@0,300..700;1,300..700&display=swap"
UA="Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"

cy_file="$ASSETS/js/cytoscape-${CYTOSCAPE_VERSION}.min.js"
dagre_file="$ASSETS/js/dagre-${DAGRE_VERSION}.min.js"
cy_dagre_file="$ASSETS/js/cytoscape-dagre-${CY_DAGRE_VERSION}.js"
elk_file="$ASSETS/js/elk-${ELK_VERSION}.bundled.js"
cy_elk_file="$ASSETS/js/cytoscape-elk-${CY_ELK_VERSION}.js"
chart_file="$ASSETS/js/chart-${CHART_VERSION}.umd.min.js"
sankey_file="$ASSETS/js/chartjs-chart-sankey-${SANKEY_VERSION}.min.js"
d3_file="$ASSETS/js/d3-${D3_VERSION}.min.js"
fonts_file="$ASSETS/css/fonts.css"

if [ -s "$cy_file" ] && [ -s "$dagre_file" ] && [ -s "$cy_dagre_file" ] &&
  [ -s "$elk_file" ] && [ -s "$cy_elk_file" ] && [ -s "$chart_file" ] &&
  [ -s "$sankey_file" ] && [ -s "$d3_file" ] && [ -s "$fonts_file" ] &&
  compgen -G "$ASSETS/fonts/*.woff2" >/dev/null; then
  echo "present: assets cached, skipping"
  exit 0
fi

# verify_sha256 FILE SUM succeeds when FILE hashes to SUM.
verify_sha256() {
  local got
  if command -v sha256sum >/dev/null 2>&1; then
    got=$(sha256sum "$1")
  else
    got=$(shasum -a 256 "$1")
  fi
  [ "${got%% *}" = "$2" ]
}

# fetch URL DEST [SHA256 [CURL_ARG...]] downloads into DEST.tmp, checks the
# pinned sum when one is given, then renames into place. A failed download
# (curl -f turns a 404 into an error) leaves nothing behind for the skip
# check above to trust.
fetch() {
  local url="$1" dest="$2" want="${3:-}"
  shift 2
  if [ $# -gt 0 ]; then shift; fi
  if ! curl -fsSL --retry 3 --retry-all-errors "$@" -o "$dest.tmp" "$url"; then
    rm -f "$dest.tmp"
    echo "present: download failed: $url" >&2
    exit 1
  fi
  if [ -n "$want" ] && ! verify_sha256 "$dest.tmp" "$want"; then
    rm -f "$dest.tmp"
    echo "present: sha256 mismatch for $url, expected $want" >&2
    exit 1
  fi
  mv "$dest.tmp" "$dest"
}

mkdir -p "$ASSETS/js" "$ASSETS/css" "$ASSETS/fonts"

echo "cytoscape@${CYTOSCAPE_VERSION}"
fetch "https://cdn.jsdelivr.net/npm/cytoscape@${CYTOSCAPE_VERSION}/dist/cytoscape.min.js" "$cy_file"

echo "dagre@${DAGRE_VERSION}"
fetch "https://cdn.jsdelivr.net/npm/dagre@${DAGRE_VERSION}/dist/dagre.min.js" "$dagre_file"

echo "cytoscape-dagre@${CY_DAGRE_VERSION}"
fetch "https://cdn.jsdelivr.net/npm/cytoscape-dagre@${CY_DAGRE_VERSION}/cytoscape-dagre.js" "$cy_dagre_file"

echo "elkjs@${ELK_VERSION}"
fetch "https://cdn.jsdelivr.net/npm/elkjs@${ELK_VERSION}/lib/elk.bundled.js" "$elk_file"

echo "cytoscape-elk@${CY_ELK_VERSION}"
fetch "https://cdn.jsdelivr.net/npm/cytoscape-elk@${CY_ELK_VERSION}/dist/cytoscape-elk.js" "$cy_elk_file"

# jsdelivr generates chart.umd.min.js itself, so it has no stable sum to pin.
echo "chart.js@${CHART_VERSION}"
fetch "https://cdn.jsdelivr.net/npm/chart.js@${CHART_VERSION}/dist/chart.umd.min.js" "$chart_file"

echo "chartjs-chart-sankey@${SANKEY_VERSION}"
fetch "https://cdn.jsdelivr.net/npm/chartjs-chart-sankey@${SANKEY_VERSION}/dist/chartjs-chart-sankey.min.js" "$sankey_file"

echo "d3@${D3_VERSION}"
fetch "https://cdn.jsdelivr.net/npm/d3@${D3_VERSION}/dist/d3.min.js" "$d3_file" "$D3_SHA256"

# The stylesheet takes its final name last, after every woff2 it names, so
# a half-finished font download never passes the skip check.
echo "google fonts (woff2)"
fonts_src="$ASSETS/css/fonts.src.css"
fetch "$FONTS_URL" "$fonts_src" "" -H "User-Agent: $UA"

urls=$(grep -oE 'https://fonts\.gstatic\.com/[^ )]+' "$fonts_src" | sort -u) || {
  echo "present: no font URLs in the fonts CSS" >&2
  exit 1
}
while read -r url; do
  name=$(basename "$url")
  echo "  $name"
  fetch "$url" "$ASSETS/fonts/$name"
done <<<"$urls"

sed 's|https://fonts\.gstatic\.com/[^)]*\/|/assets/fonts/|g' "$fonts_src" >"$fonts_file.tmp"
mv "$fonts_file.tmp" "$fonts_file"
rm -f "$fonts_src"

echo "done → $ASSETS"
