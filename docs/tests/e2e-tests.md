# E2E Tests

## Purpose

E2E tests validate the **full Kubernetes COSI stack**: COSI controller,
provisioner sidecar, our driver, and VersityGW, all deployed in a real
cluster. They prove that:

- COSI Custom Resources (BucketClass, BucketClaim, BucketAccessClass,
  BucketAccess) create and reconcile correctly
- The sidecar correctly calls our driver's gRPC RPCs
- Kubernetes Secrets are created with valid credentials
- Pods can consume the Secrets and perform S3 operations
- Cleanup (delete BucketAccess/BucketClaim) properly removes backend resources
- Error propagation to CRD status fields
- Driver/controller recovery after restart

These tests are the primary defense against **Kubernetes version and COSI
controller/sidecar regressions**.

## Framework

The suite is written in **Kyverno Chainsaw** (declarative YAML), aligning
with the upstream COSI project's own conformance test pattern at
`kubernetes-sigs/container-object-storage-interface/test/e2e`. Chainsaw
provides built-in retry semantics that tolerate transient COSI v0.2.2
reconciliation delays better than imperative Go polling.

**Layout:**

```
test/chainsaw/
├── kind-config.yaml          # kind cluster definition
├── chainsaw-config.yaml      # Chainsaw timeouts
├── values.yaml               # Shared bindings (driverName, image names, ...)
├── bootstrap/                # One-time cluster setup
│   ├── cosi-controller-install.sh
│   ├── versitygw.yaml
│   ├── bucketclass-*.yaml
│   └── bucketaccessclass-*.yaml
├── verifier/                 # Custom Pod image (amazon/aws-cli + jq)
│   ├── Dockerfile
│   ├── entrypoint.sh         # user-credentials ops
│   └── admin.sh              # root-credentials ops
├── tests/                    # Parallel-safe test cases (25)
├── recovery/                 # Serial recovery tests (2)
├── setup.sh
└── teardown.sh
```

## Running

```bash
# Requires: docker, kind, kubectl, helm, curl, chainsaw v0.2.14
# (chainsaw installed via `make install-chainsaw`)

# One-shot (setup + parallel suite + recovery suite + teardown)
make test-e2e-all

# Iterative local dev
make test-e2e-setup
make test-e2e           # parallel suite
make test-e2e-recovery  # serial recovery suite
make test-e2e-teardown

# Debug: keep cluster after failure
make test-e2e-keep
# (inspect with kubectl)
make test-e2e-teardown
```

On Arch/Manjaro with rootless Docker, export `DOCKER_HOST=tcp://localhost:2375`.

## Verifier image

In-cluster Pods that perform S3 operations share a single image:
`versitygw-cosi-verifier:e2e`, built from `test/chainsaw/verifier/Dockerfile`.

Two modes:

- **entrypoint.sh** (user mode) — mounts a BucketAccess Secret at
  `/conf/BucketInfo`, extracts credentials, and performs ops
  (`head-bucket`, `put-object`, `get-object`, `list-objects`,
  `expect-403`, `extract-access-key`).
- **admin.sh** (admin mode) — reads root credentials from env vars, performs
  cluster-scope S3/Admin ops (`head-bucket-by-name`, `head-bucket-not-found`,
  `head-object`, `put-object`, `delete-bucket-by-name`, `list-users`,
  `assert-user-absent`).

## Test Cases

The 27 test cases are unchanged from the original specification. Each TC-E-*
maps to one Chainsaw test directory.

### Lifecycle (`test/chainsaw/tests/tc-e-00*`)

- **TC-E-001** — BucketClass creation succeeds
- **TC-E-002** — BucketClaim creates a Bucket CRD
- **TC-E-003** — BucketClaim.status.bucketName set
- **TC-E-004** — BucketAccessClass creation succeeds
- **TC-E-005** — BucketAccess yields Secret
- **TC-E-006** — Secret contains accountId (accessKeyID)
- **TC-E-007** — Secret + user removed on BucketAccess delete
- **TC-E-008** — Revoked credentials return 403
- **TC-E-009** — BucketClaim deletion reaches K8s completion (VersityGW
  bucket cleanup via admin API — COSI v0.2.2 controller cannot drive
  `DriverDeleteBucket` within the test budget under parallel load; see
  upstream #79)
- **TC-E-010** — Full lifecycle in order

### Secret validation (`test/chainsaw/tests/tc-e-02*`)

- **TC-E-020** — Secret in BucketAccess namespace
- **TC-E-021** — BucketInfo has endpoint + bucketName
- **TC-E-022** — BucketInfo has accessKeyID + accessSecretKey
- **TC-E-023** — Credentials connect via HeadBucket

### Pod consumption (`test/chainsaw/tests/tc-e-03*`)

Chainsaw runs actual Pods (via Jobs) that mount the Secret.

- **TC-E-030** — Pod can PutObject
- **TC-E-031** — Pod can GetObject
- **TC-E-032** — Pod can ListObjectsV2

### Error scenarios (`test/chainsaw/tests/tc-e-04*`)

- **TC-E-040** — Missing BucketClass
- **TC-E-041** — IAM auth rejected
- **TC-E-042** — Missing BucketClaim
- **TC-E-043** — Driver rejects unknown parameters
- **TC-E-044** — Graceful cleanup when BucketClaim deleted while BucketAccess exists

### Recovery (`test/chainsaw/recovery/tc-e-05*`)

Serial-only (separate Chainsaw invocation via `make test-e2e-recovery`).

- **TC-E-050** — Driver pod restart
- **TC-E-051** — COSI controller restart

### Multi-access (`test/chainsaw/tests/tc-e-06*`)

- **TC-E-060** — Two accesses, cross-read both directions
- **TC-E-061** — Delete one, other still works
- **TC-E-062** — Retain policy keeps bucket

---

## Chainsaw Conventions

- **Bindings**: each test declares `spec.bindings` that pull from `values.yaml`.
  See `test/chainsaw/values.yaml` for the shared binding catalog.
- **Ephemeral namespace**: Chainsaw creates a fresh `chainsaw-<random>`
  namespace per test; tests omit `metadata.namespace` and let Chainsaw inject it.
- **Assert vs error**: `assert` polls until the resource matches; `error`
  polls until it either does NOT match or the resource is gone. Use
  `error` for "must never be Ready" or "must be deleted" scenarios.
- **Script steps**: only for procedural glue (e.g., snapshotting a Secret
  into a ConfigMap before deletion, restarting a deployment). S3 assertions
  run inside Jobs using the verifier image.

## Version Compatibility

- **COSI**: v0.2.2 (v1alpha1 APIs). The upstream `main` has moved to
  v1alpha2, which this driver does NOT yet support.
- **Kubernetes**: whatever `kind` ships with by default (typically latest minor).
- **VersityGW**: `versity/versitygw:latest` (pulled at test time).

## Summary

| ID Range         | Area                    | Count |
|------------------|-------------------------|-------|
| TC-E-001 ~ 010   | COSI resource lifecycle | 10    |
| TC-E-020 ~ 023   | Secret validation       | 4     |
| TC-E-030 ~ 032   | Pod consumption         | 3     |
| TC-E-040 ~ 044   | Error scenarios         | 5     |
| TC-E-050 ~ 051   | Recovery scenarios      | 2     |
| TC-E-060 ~ 062   | Multi-access scenarios  | 3     |
| **Total**        |                         | **27**|
