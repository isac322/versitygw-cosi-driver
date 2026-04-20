#!/usr/bin/env bash
# Unified cleanup invoked from each Chainsaw test's step-level `cleanup:`
# block. Handles the COSI v0.2.2 finalizer-stranding bug (upstream issues
# #79, #227) by retrying the finalizer strip until the resources are
# actually garbage-collected — under parallel load the controller can
# re-add a finalizer before K8s GC runs, so a single-pass patch races.
#
# Chainsaw guarantees step-level cleanup blocks run at test end regardless
# of success or failure, so calling this script reliably cleans up orphans
# left by a failing step.
#
# Modes:
#   simple <ns>  — read-only tests that only assert cluster-scoped
#                  resources; just delete the namespace.
#   full <ns>    — tests that create BucketClaim/BucketAccess (most
#                  tests): strip finalizers on claims, accesses, and
#                  Buckets owned by $ns; delete the namespace; force-
#                  finalize if it gets stuck Terminating.
#   recovery <ns> <driver_ns> <driver_deploy>
#                — same as full, but first waits up to 60s for the
#                  driver/controller deployment to be Available (the
#                  test may have just restarted it). Stripping
#                  finalizers while the driver is still rolling can
#                  cause false "delete RPC never called" leaks.
set +e

MODE="${1:-}"
NS="${2:-}"

if [ -z "$MODE" ] || [ -z "$NS" ]; then
  echo "usage: cleanup.sh <simple|full|recovery> <namespace> [driver_ns driver_deploy]" >&2
  exit 1
fi

force_finalize_ns() {
  # If namespace is stuck Terminating after 5s, remove its finalizer via
  # the /finalize subresource (the only way to unstick a namespace whose
  # contents still carry finalizers the controller can't clear). The
  # delay gives the legitimate K8s GC path a window to finish first.
  sleep 5
  if kubectl get ns "$NS" 2>/dev/null | grep -q Terminating; then
    kubectl get ns "$NS" -o json 2>/dev/null | jq '.spec.finalizers=[]' | \
      kubectl replace --raw "/api/v1/namespaces/$NS/finalize" -f - >/dev/null 2>&1 || true
  fi
}

case "$MODE" in
  simple)
    kubectl delete namespace "$NS" --wait=false --ignore-not-found 2>/dev/null || true
    force_finalize_ns
    exit 0
    ;;
  full)
    ;;
  recovery)
    # Wait up to 60s for the restarted deployment(s) to be Available
    # before stripping finalizers — otherwise a mid-restart controller
    # can fail to drive the delete path entirely. Two forms supported:
    #   recovery <ns> <driver_ns> <driver_deploy>   — explicit target
    #   recovery <ns>                               — probe known COSI
    #                                                 controller layouts
    # (v0.2.x: container-object-storage-system; older:
    # objectstorage-provisioner-system). The probing variant is used by
    # the COSI-controller-restart test, which itself probes both
    # locations; keeping the same probe here avoids divergence.
    DRIVER_NS="${3:-}"
    DRIVER_DEPLOY="${4:-}"
    if [ -n "$DRIVER_NS" ] && [ -n "$DRIVER_DEPLOY" ]; then
      # Hard gate: if rollout status fails (timeout / api unreachable),
      # log it so we don't silently strip finalizers against a sick
      # controller. Exit status is intentionally not propagated so
      # cleanup still runs — stripping with a dead controller is strictly
      # better than leaving orphans for the next test.
      kubectl rollout status "deployment/$DRIVER_DEPLOY" -n "$DRIVER_NS" --timeout=60s \
        || echo "cleanup.sh: rollout status for $DRIVER_NS/$DRIVER_DEPLOY did not become Available within 60s; proceeding anyway" >&2
    else
      found=0
      for pair in \
        "container-object-storage-system container-object-storage-controller" \
        "objectstorage-provisioner-system objectstorage-provisioner-controller-manager"; do
        set -- $pair
        if kubectl get deployment "$2" -n "$1" >/dev/null 2>&1; then
          found=1
          kubectl rollout status "deployment/$2" -n "$1" --timeout=60s \
            || echo "cleanup.sh: rollout status for $1/$2 did not become Available within 60s; proceeding anyway" >&2
        fi
      done
      if [ "$found" -eq 0 ]; then
        echo "cleanup.sh: no known COSI controller deployment found to wait on; proceeding anyway" >&2
      fi
    fi
    ;;
  *)
    echo "unknown mode: $MODE" >&2
    exit 1
    ;;
esac

# --- full / recovery shared path ---

# Issue delete for namespaced COSI resources; controller should remove
# finalizers naturally. --wait=false so we can immediately observe and
# patch if it stalls.
kubectl delete bucketaccess --all -n "$NS" --wait=false --ignore-not-found 2>/dev/null
kubectl delete bucketclaim --all -n "$NS" --wait=false --ignore-not-found 2>/dev/null

# Retry loop: up to ~15s. Each iteration strips finalizers on every
# surviving COSI resource owned by $NS, then checks if they're gone.
# Under parallel load the controller re-adds finalizers on reconcile,
# so a single pass loses this race.
#
# We require at least 5 iterations before allowing the early-exit
# path, because the controller may create a Bucket CRD lazily (e.g.,
# after the test's BucketClaim deletion request has already been
# observed). If we break too early, the Bucket arrives after cleanup
# exits and lingers in the cluster as an orphan.
MIN_ITERS=5
for i in $(seq 1 15); do
  # Cluster-scoped Bucket: strip finalizer + explicit delete. Bucket CRDs
  # carry bucketClaim.namespace to identify ownership.
  for b in $(kubectl get buckets -o name 2>/dev/null); do
    owner=$(kubectl get "$b" -o jsonpath='{.spec.bucketClaim.namespace}' 2>/dev/null)
    if [ "$owner" = "$NS" ]; then
      kubectl patch "$b" --type=merge -p '{"metadata":{"finalizers":null}}' 2>/dev/null || true
      kubectl delete "$b" --ignore-not-found --wait=false 2>/dev/null || true
    fi
  done
  for claim in $(kubectl get bucketclaim -n "$NS" -o name 2>/dev/null); do
    kubectl patch "$claim" -n "$NS" --type=merge -p '{"metadata":{"finalizers":null}}' 2>/dev/null || true
  done
  for ba in $(kubectl get bucketaccess -n "$NS" -o name 2>/dev/null); do
    kubectl patch "$ba" -n "$NS" --type=merge -p '{"metadata":{"finalizers":null}}' 2>/dev/null || true
  done

  # Exit early if everything is gone (including any stale cluster-scoped
  # Buckets still owned by $NS) AND we've run at least MIN_ITERS
  # iterations so late-arriving Bucket CRDs are caught.
  remaining=0
  remaining=$(( remaining + $(kubectl get bucketclaim,bucketaccess -n "$NS" -o name 2>/dev/null | wc -l) ))
  for b in $(kubectl get buckets -o name 2>/dev/null); do
    owner=$(kubectl get "$b" -o jsonpath='{.spec.bucketClaim.namespace}' 2>/dev/null)
    [ "$owner" = "$NS" ] && remaining=$((remaining+1))
  done
  if [ "$remaining" -eq 0 ] && [ "$i" -ge "$MIN_ITERS" ]; then
    break
  fi
  sleep 1
done

kubectl delete namespace "$NS" --wait=false --ignore-not-found 2>/dev/null || true
force_finalize_ns
exit 0
