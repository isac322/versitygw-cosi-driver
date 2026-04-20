# Cleanup workaround for COSI v0.2.2 finalizer bug

## Problem

COSI v0.2.2 controller has a race in finalizer removal: when a BucketAccess
is deleted concurrently with its Secret (Chainsaw's default cleanup deletes
all resources in a namespace in parallel), the controller fails to remove
the `cosi.objectstorage.k8s.io/bucketaccess-bucket-protection` finalizer
from the associated Bucket CRD. This cascades to:

- Bucket CRD stuck with finalizer
- BucketClaim's `bucketclaim-protection` finalizer can't be removed
- Namespace termination blocks indefinitely
- `chainsaw` test fails with `context deadline exceeded` on cleanup

## Related upstream issues

- <https://github.com/kubernetes-sigs/container-object-storage-interface/issues/79>
  (CLOSED, but the exact finalizer `bucketaccess-bucket-protection` is named
  in the original report; edge cases persist in v0.2.2)
- <https://github.com/kubernetes-sigs/container-object-storage-interface/issues/227>
  (OPEN, gRPC resource provision-deprovision race condition)
- <https://github.com/kubernetes-sigs/container-object-storage-interface/issues/254>
  (OPEN, Bucket watcher missing; cleanup uses exponential backoff)
- <https://github.com/kubernetes-sigs/container-object-storage-interface/issues/285>
  (OPEN, `status.bucketName` not set under delay)

## Workaround applied in tests

Each test that creates a BucketAccess includes an explicit `cleanup:` step
that:

1. Deletes BucketAccess (best-effort, ignores not-found).
2. Waits briefly for the sidecar to process revoke.
3. Strips the `bucketaccess-bucket-protection` finalizer from any Bucket
   left with only that finalizer (forces controller to let it go).
4. Deletes BucketClaim.
5. Strips remaining finalizers from both Bucket and BucketClaim as a last
   resort if the controller still hasn't progressed after 10s.

This is a test-only workaround; the driver itself is not affected because
the driver manages only its own S3/user state, not Kubernetes finalizers.
