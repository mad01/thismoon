#!/bin/bash
# Run the cluster-gated tests against the present deployment in a kind
# cluster: the store conformance suite straight against the API server, and
# the HTTP end-to-end test through a port-forward into the service. Expects
# `kubectl apply -k deploy/overlays/kind` to have rolled out already (make
# kind-test does that). Never creates or deletes a cluster.
set -euo pipefail

NAMESPACE="${PRESENT_E2E_NAMESPACE:-present}"
LOCAL_PORT="${PRESENT_E2E_PORT:-17425}"
cd "$(dirname "$0")/.."

kubectl -n "$NAMESPACE" port-forward svc/present "${LOCAL_PORT}:7423" >/tmp/present-e2e-port-forward.log 2>&1 &
pf=$!
trap 'kill "$pf" 2>/dev/null || true' EXIT

for _ in $(seq 1 40); do
  if curl -fsS "http://127.0.0.1:${LOCAL_PORT}/version" >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done
curl -fsS "http://127.0.0.1:${LOCAL_PORT}/version" >/dev/null || {
  echo "present: port-forward to svc/present in $NAMESPACE never answered" >&2
  cat /tmp/present-e2e-port-forward.log >&2
  exit 1
}

PRESENT_KIND_TEST=1 PRESENT_E2E_URL="http://127.0.0.1:${LOCAL_PORT}" \
  go test -count=1 -v ./internal/store/k8sstore/ ./internal/e2e/
