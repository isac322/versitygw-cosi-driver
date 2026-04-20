#!/usr/bin/env bash
# Install COSI v1alpha1 CRDs + controller from pre-rendered manifests.
#
# The CRDs ship with the `api-approved.kubernetes.io` annotation already
# injected (required by Kubernetes >=1.35 for *.k8s.io API groups). The
# controller manifest is a kustomize render of the upstream controller
# overlay at the COSI_VERSION pinned in cosi-controller-regen.sh.
#
# To bump to a new COSI version, run ./cosi-controller-regen.sh (requires
# network + kubectl kustomize) and commit the regenerated manifests.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo ">>> Applying pre-rendered COSI CRDs"
kubectl apply -f "$SCRIPT_DIR/cosi-v0.2.2-crds.yaml"

echo ">>> Applying pre-rendered COSI controller"
kubectl apply -f "$SCRIPT_DIR/cosi-v0.2.2-controller.yaml"

echo ">>> Waiting for COSI controller to be Ready"
kubectl wait --for=condition=available --timeout=120s \
  deployment/container-object-storage-controller \
  -n container-object-storage-system
