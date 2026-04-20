#!/usr/bin/env bash
# Teardown: destroy the kind cluster + remove the project-local kubeconfig
# written by setup.sh. Never touches $HOME/.kube/config.
set -euo pipefail

CLUSTER="${KIND_CLUSTER:-versitygw-cosi-chainsaw}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

export KUBECONFIG="${KUBECONFIG:-$REPO_ROOT/.e2e-kubeconfig}"

echo ">>> Delete kind cluster '$CLUSTER' (KUBECONFIG=$KUBECONFIG)"
kind delete cluster --name "$CLUSTER" --kubeconfig "$KUBECONFIG"

# Remove the project-local kubeconfig so nothing stale remains.
rm -f "$KUBECONFIG"
