#!/usr/bin/env bash
# Regenerate the pre-rendered COSI manifests used by cosi-controller-install.sh.
# Requires network access and `kubectl kustomize`. Run after bumping
# COSI_VERSION; commit the resulting cosi-v<version>-crds.yaml and
# cosi-v<version>-controller.yaml, then update cosi-controller-install.sh to
# point at the new file names.
#
# Rationale: doing this work once at regen time (seconds) keeps every e2e
# setup fast instead of paying curl × 5 + kustomize build on every run.
set -euo pipefail

COSI_VERSION="${COSI_VERSION:-v0.2.2}"
BASE_URL="https://raw.githubusercontent.com/kubernetes-sigs/container-object-storage-interface/${COSI_VERSION}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

CRDS=(
  "objectstorage.k8s.io_bucketaccesses.yaml"
  "objectstorage.k8s.io_bucketaccessclasses.yaml"
  "objectstorage.k8s.io_bucketclaims.yaml"
  "objectstorage.k8s.io_bucketclasses.yaml"
  "objectstorage.k8s.io_buckets.yaml"
)

CRDS_OUT="$SCRIPT_DIR/cosi-${COSI_VERSION}-crds.yaml"
CTRL_OUT="$SCRIPT_DIR/cosi-${COSI_VERSION}-controller.yaml"

echo ">>> Rendering CRDs with api-approved.kubernetes.io annotation → $CRDS_OUT"
{
  for crd in "${CRDS[@]}"; do
    curl -fsSL "${BASE_URL}/client/config/crd/${crd}" \
      | sed '0,/^  annotations:$/{s|^  annotations:$|  annotations:\n    api-approved.kubernetes.io: "https://github.com/kubernetes-sigs/container-object-storage-interface"|}'
    echo '---'
  done
} > "$CRDS_OUT"

echo ">>> Rendering controller kustomize overlay → $CTRL_OUT"
kubectl kustomize "https://github.com/kubernetes-sigs/container-object-storage-interface/controller?ref=${COSI_VERSION}" \
  > "$CTRL_OUT"

echo ""
echo "Done. If the COSI version changed, update cosi-controller-install.sh to"
echo "reference the new manifest file names, then commit:"
echo "  - $CRDS_OUT"
echo "  - $CTRL_OUT"
