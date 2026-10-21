#!/bin/bash
# agents.json is derived from skills/factory/agents/*.md and committed, since
# the recipe links it from the sources cache. Fail when the two have drifted.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

python3 "$SCRIPT_DIR/render_agents_json.py" --check
echo "agents-json-test: agents.json matches the worker definitions"
