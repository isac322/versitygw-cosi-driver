#!/usr/bin/env bash
# Teardown: destroy the kind cluster.
set -euo pipefail

CLUSTER="${KIND_CLUSTER:-versitygw-cosi-chainsaw}"

echo ">>> Delete kind cluster '$CLUSTER'"
kind delete cluster --name "$CLUSTER"
