#!/bin/bash
set -euo pipefail

WORKDIR="${PRESENT_WORKDIR:-$HOME/.config/present}"
ASSETS="$WORKDIR/assets"
CYTOSCAPE_VERSION="3.31.0"
DAGRE_VERSION="0.8.5"
CY_DAGRE_VERSION="2.5.0"
CHART_VERSION="4.4.6"
FONTS_URL="https://fonts.googleapis.com/css2?family=Fira+Code:wght@300..700&family=Inter:ital,opsz,wght@0,14..32,300..700;1,14..32,300..700&family=Lexend:wght@300..700&family=Work+Sans:ital,wght@0,300..700;1,300..700&display=swap"
UA="Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"

cy_file="$ASSETS/js/cytoscape-${CYTOSCAPE_VERSION}.min.js"
dagre_file="$ASSETS/js/dagre-${DAGRE_VERSION}.min.js"
cy_dagre_file="$ASSETS/js/cytoscape-dagre-${CY_DAGRE_VERSION}.js"
chart_file="$ASSETS/js/chart-${CHART_VERSION}.umd.min.js"
fonts_file="$ASSETS/css/fonts.css"

if [ -s "$cy_file" ] && [ -s "$dagre_file" ] && [ -s "$cy_dagre_file" ] && [ -s "$chart_file" ] && [ -s "$fonts_file" ] && [ "$(ls -1 "$ASSETS/fonts/"*.woff2 2>/dev/null | wc -l)" -gt 0 ]; then
  echo "present: assets cached, skipping"
  exit 0
fi

mkdir -p "$ASSETS/js" "$ASSETS/css" "$ASSETS/fonts"

echo "cytoscape@${CYTOSCAPE_VERSION}"
curl -sL "https://cdn.jsdelivr.net/npm/cytoscape@${CYTOSCAPE_VERSION}/dist/cytoscape.min.js" \
  -o "$cy_file"

echo "dagre@${DAGRE_VERSION}"
curl -sL "https://cdn.jsdelivr.net/npm/dagre@${DAGRE_VERSION}/dist/dagre.min.js" \
  -o "$dagre_file"

echo "cytoscape-dagre@${CY_DAGRE_VERSION}"
curl -sL "https://cdn.jsdelivr.net/npm/cytoscape-dagre@${CY_DAGRE_VERSION}/cytoscape-dagre.js" \
  -o "$cy_dagre_file"

echo "chart.js@${CHART_VERSION}"
curl -sL "https://cdn.jsdelivr.net/npm/chart.js@${CHART_VERSION}/dist/chart.umd.min.js" \
  -o "$chart_file"

echo "google fonts (woff2)"
curl -sL -H "User-Agent: $UA" "$FONTS_URL" -o "$fonts_file"

grep -oE 'https://fonts\.gstatic\.com/[^ )]+' "$fonts_file" | sort -u | while read -r url; do
  name=$(basename "$url")
  echo "  $name"
  curl -sL "$url" -o "$ASSETS/fonts/$name"
done

tmpf=$(mktemp)
sed 's|https://fonts\.gstatic\.com/[^)]*\/|/assets/fonts/|g' "$fonts_file" > "$tmpf"
mv "$tmpf" "$fonts_file"

echo "done → $ASSETS"
