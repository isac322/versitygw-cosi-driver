#!/usr/bin/env bash
# E2E cluster diagnostics. Invoked before teardown when tests fail, so that
# transient CI failures (e.g. upstream COSI controller races) can be
# root-caused from cluster state that would otherwise be lost after the
# kind cluster is deleted.
set +e
export KUBECONFIG="${KUBECONFIG:-$(pwd)/.e2e-kubeconfig}"

echo '=== kind clusters ==='
kind get clusters

echo '=== all pods (wide) ==='
kubectl get pods -A -o wide

echo '=== recent events (last 100) ==='
kubectl get events -A --sort-by=.lastTimestamp | tail -100

echo '=== all jobs ==='
kubectl get jobs -A

echo '=== describe non-Running/non-Succeeded pods ==='
kubectl get pods -A --field-selector=status.phase!=Running,status.phase!=Succeeded \
  -o jsonpath='{range .items[*]}{.metadata.namespace}/{.metadata.name}{"\n"}{end}' \
  | while IFS=/ read -r ns name; do
      [ -z "$ns" ] && continue
      echo "--- describe $ns/pod/$name ---"
      kubectl describe pod "$name" -n "$ns"
    done

echo '=== chainsaw-* namespace pod logs (current + previous) ==='
kubectl get ns -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' \
  | grep '^chainsaw-' \
  | while IFS= read -r ns; do
      kubectl get pods -n "$ns" -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' \
        | while IFS= read -r pod; do
            [ -z "$pod" ] && continue
            echo "--- $ns/$pod (current) ---"
            kubectl logs -n "$ns" "$pod" --all-containers --tail=500
            echo "--- $ns/$pod (previous) ---"
            kubectl logs -n "$ns" "$pod" --all-containers --previous --tail=500 2>/dev/null
          done
    done

echo '=== driver logs ==='
kubectl logs -n versitygw-cosi-driver-system -l app.kubernetes.io/name=versitygw-cosi-driver --all-containers --tail=500

echo '=== cosi controller logs ==='
kubectl logs -n container-object-storage-system -l app.kubernetes.io/part-of=container-object-storage-interface --tail=500

echo '=== versitygw logs ==='
kubectl logs -n versitygw-system -l app=versitygw --tail=500
