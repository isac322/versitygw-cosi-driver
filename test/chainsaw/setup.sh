#!/usr/bin/env bash
# Setup: create kind cluster + install COSI + deploy VersityGW + build/load
# driver image + Helm install driver + build/load verifier image + apply
# cluster-scoped BucketClass/BucketAccessClass resources.
#
# Idempotent-ish: best-effort; re-running after partial failure may require
# `./test/chainsaw/teardown.sh` first.
#
# Writes a project-local kubeconfig at $REPO_ROOT/.e2e-kubeconfig and
# exports KUBECONFIG so kind/kubectl/helm/chainsaw do NOT touch
# $HOME/.kube/config. Caller shells that want to interact with the test
# cluster must `export KUBECONFIG=<repo>/.e2e-kubeconfig` first.
set -euo pipefail

CLUSTER="${KIND_CLUSTER:-versitygw-cosi-chainsaw}"
DRIVER_IMAGE="versitygw-cosi-driver:e2e"
VERIFIER_IMAGE="versitygw-cosi-verifier:e2e"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

export KUBECONFIG="${KUBECONFIG:-$REPO_ROOT/.e2e-kubeconfig}"

echo ">>> [1/9] Create kind cluster '$CLUSTER' (KUBECONFIG=$KUBECONFIG)"
# Delete any stale cluster with the same name (e.g. left behind by a
# previous run that crashed before teardown). Makes the target idempotent
# so `make test-e2e-all` can always start from a clean slate.
kind delete cluster --name "$CLUSTER" --kubeconfig "$KUBECONFIG" 2>/dev/null || true
kind create cluster --name "$CLUSTER" --config "$SCRIPT_DIR/kind-config.yaml" --kubeconfig "$KUBECONFIG"

echo ">>> [2/9] Install COSI controller (v0.2.2)"
"$SCRIPT_DIR/bootstrap/cosi-controller-install.sh"

echo ">>> [3/9] Deploy VersityGW"
kubectl apply -f "$SCRIPT_DIR/bootstrap/versitygw.yaml"
kubectl wait --for=condition=available --timeout=120s \
  deployment/versitygw -n versitygw-system

echo ">>> [4/9] Build driver image"
# Use buildx + GitHub Actions cache on CI (saves ~45s on cache hit), fall back
# to a plain docker build locally or when no buildx builder is configured.
if [ "${GITHUB_ACTIONS:-}" = "true" ] && docker buildx inspect >/dev/null 2>&1; then
  docker buildx build --load \
    --cache-from=type=gha \
    --cache-to=type=gha,mode=max \
    -t "$DRIVER_IMAGE" "$REPO_ROOT"
else
  docker build -t "$DRIVER_IMAGE" "$REPO_ROOT"
fi

echo ">>> [5/9] Load driver image into kind"
kind load docker-image "$DRIVER_IMAGE" --name "$CLUSTER"

echo ">>> [6/9] Load verifier image into kind"
# Default: pull the prebuilt image from GHCR (release-verifier.yaml workflow
# publishes it on master changes under test/chainsaw/verifier/**). Falls back
# to a local build if the pull fails (first run before the image exists, or
# an offline developer environment). Override with VERIFIER_SRC=build to
# force a rebuild regardless.
VERIFIER_SRC="${VERIFIER_SRC:-ghcr.io/isac322/versitygw-cosi-driver/verifier:latest}"
if [ "$VERIFIER_SRC" = "build" ] || ! docker pull "$VERIFIER_SRC" 2>/dev/null; then
  [ "$VERIFIER_SRC" = "build" ] \
    || echo ">>> pull '$VERIFIER_SRC' failed, building locally"
  docker build -t "$VERIFIER_IMAGE" "$SCRIPT_DIR/verifier"
else
  docker tag "$VERIFIER_SRC" "$VERIFIER_IMAGE"
fi
kind load docker-image "$VERIFIER_IMAGE" --name "$CLUSTER"

echo ">>> [7/9] Copy root credentials to driver namespace"
kubectl create namespace versitygw-cosi-driver-system --dry-run=client -o yaml | kubectl apply -f -
# Re-emit with namespace rewritten.
kubectl get secret versitygw-root-credentials -n versitygw-system -o yaml \
  | sed 's/namespace: versitygw-system/namespace: versitygw-cosi-driver-system/' \
  | grep -v '^  resourceVersion:\|^  uid:\|^  creationTimestamp:' \
  | kubectl apply -f -

echo ">>> [8/9] Install driver via Helm"
helm upgrade --install versitygw-cosi-driver-e2e "$REPO_ROOT/deploy/helm/versitygw-cosi-driver" \
  --namespace versitygw-cosi-driver-system \
  --set driver.name=versitygw.cosi.dev \
  --set driver.image.repository=versitygw-cosi-driver \
  --set driver.image.tag=e2e \
  --set driver.image.pullPolicy=Never \
  --set versitygw.serviceName=versitygw.versitygw-system.svc.cluster.local \
  --set versitygw.credentials.secretName=versitygw-root-credentials \
  --set bucketClass.create=false \
  --set bucketAccessClass.create=false \
  --wait --timeout 120s

echo ">>> [9/9] Apply shared cluster-scoped BucketClass/BucketAccessClass"
kubectl apply -f "$SCRIPT_DIR/bootstrap/bucketclass-default.yaml"
kubectl apply -f "$SCRIPT_DIR/bootstrap/bucketclass-retain.yaml"
kubectl apply -f "$SCRIPT_DIR/bootstrap/bucketclass-invalid.yaml"
kubectl apply -f "$SCRIPT_DIR/bootstrap/bucketaccessclass-default.yaml"
kubectl apply -f "$SCRIPT_DIR/bootstrap/bucketaccessclass-iam.yaml"

echo ""
echo "Cluster '$CLUSTER' is ready for Chainsaw tests."
echo "Run: make test-e2e"
