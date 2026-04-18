#!/usr/bin/env bash
# Install COSI v1alpha1 CRDs + controller into the current kube context.
#
# COSI v0.2.2 CRDs lack the `api-approved.kubernetes.io` annotation required
# by Kubernetes >=1.35 for *.k8s.io API groups, so we inject it before apply.
# This mirrors the logic in the legacy Go E2E setup (test/e2e/setup_test.go).
set -euo pipefail

COSI_VERSION="${COSI_VERSION:-v0.2.2}"
BASE_URL="https://raw.githubusercontent.com/kubernetes-sigs/container-object-storage-interface/${COSI_VERSION}"

CRDS=(
  "objectstorage.k8s.io_bucketaccesses.yaml"
  "objectstorage.k8s.io_bucketaccessclasses.yaml"
  "objectstorage.k8s.io_bucketclaims.yaml"
  "objectstorage.k8s.io_bucketclasses.yaml"
  "objectstorage.k8s.io_buckets.yaml"
)

echo ">>> Installing COSI ${COSI_VERSION} CRDs (with api-approved annotation injection)"
for crd in "${CRDS[@]}"; do
  url="${BASE_URL}/client/config/crd/${crd}"
  curl -fsSL "$url" \
    | sed '0,/^  annotations:$/{s|^  annotations:$|  annotations:\n    api-approved.kubernetes.io: "https://github.com/kubernetes-sigs/container-object-storage-interface"|}' \
    | kubectl apply -f -
done

echo ">>> Installing COSI ${COSI_VERSION} controller via kustomize"
kubectl apply -k "https://github.com/kubernetes-sigs/container-object-storage-interface/controller?ref=${COSI_VERSION}"

echo ">>> Waiting for COSI controller to be Ready"
kubectl wait --for=condition=available --timeout=120s \
  deployment/container-object-storage-controller \
  -n container-object-storage-system
