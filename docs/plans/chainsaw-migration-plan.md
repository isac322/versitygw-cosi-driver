# E2E Test Migration Plan: Go `e2e-framework` → Kyverno Chainsaw

**Status:** REVISED — Codex rescue cross-validation applied 2026-04-18
**Author:** Claude (with Byeonghoon Yoo)
**Created:** 2026-04-18
**Last revision:** 2026-04-18 (post-Codex review)
**Scope:** Replace the Go-based E2E suite under `test/e2e/` with Chainsaw-based declarative tests, aligned with upstream COSI conformance practice, while preserving every existing test case and its assertion semantics.

---

## 1. Goals & Non-Goals

### 1.1 Goals

1. Replace Go E2E implementation (`test/e2e/*.go`, 2262 lines) with Chainsaw YAML tests.
2. Preserve all 27 test cases (TC-E-001 … TC-E-062) with equivalent or stricter assertions (see Section 13 parity table).
3. Achieve COSI upstream alignment: use the same Chainsaw pattern as `kubernetes-sigs/container-object-storage-interface/test/e2e`. (Actual upstream conformance test is a **separate follow-up**, see Section 11.2.)
4. Run 25 functional tests in parallel on a single kind cluster with namespace isolation; run 2 recovery tests (TC-E-050, TC-E-051) serially in a separate Chainsaw invocation (they restart shared infrastructure and cannot share a cluster with concurrent tests).
5. Narrow the observed COSI v0.2.2 race-condition exposure (`bucketReady=true` with empty `bucketName`) by using atomic Chainsaw `assert` steps that require both fields in one poll. **Not a claim of full fix** — controller-side root cause is outside scope. See Risk in Section 9.1.
6. Maintain existing CI coverage and add a new `e2e` job that runs the Chainsaw suite (label-gated initially).
7. Keep all non-E2E tests (unit/component/integration) untouched.

### 1.2 Non-Goals

1. Adding new test cases. Scope is strict migration of existing 27 scenarios.
2. Reworking unit, component, or integration layers.
3. Upgrading COSI spec from v1alpha1 to v1alpha2.
4. Merging Chainsaw into the `go test` invocation — Chainsaw is a separate CLI.
5. Replacing the Helm chart or driver image pipeline.
6. Performance optimization beyond parallel execution.

### 1.3 Success Criteria

- `make test-e2e` runs Chainsaw against a freshly created kind cluster and all 27 test cases pass consistently (≥3 consecutive runs) with parallel execution.
- CI pipeline includes an E2E job (optional: gated to a specific trigger to avoid always-running long tests).
- `docs/tests/e2e-tests.md` updated to reflect Chainsaw-based layout and commands.
- `CHANGELOG.md` updated.
- Zero regressions in unit/component/integration suites.
- Old Go E2E code fully removed (no dead `_test.go` under `test/e2e/`).

---

## 2. Current State Analysis

### 2.1 Go E2E File Inventory

| File | Lines | Purpose |
|------|-------|---------|
| `test/e2e/setup_test.go` | 411 | TestMain: kind cluster + COSI install + VersityGW deploy + driver Helm install + waitForAllReady + cluster-scoped resource apply. Finish: teardown. |
| `test/e2e/helpers_test.go` | 556 | kubectl wrappers, S3 client builders, port-forward helper, BucketInfo parser, credential extractor, admin client, bucket/user verification. |
| `test/e2e/lifecycle_test.go` | 377 | TC-E-001 … TC-E-010 (10 tests). |
| `test/e2e/secret_test.go` | 175 | TC-E-020 … TC-E-023 (4 tests). |
| `test/e2e/pod_test.go` | 192 | TC-E-030 … TC-E-032 (3 tests — currently simulated via port-forward, NOT real Pods). |
| `test/e2e/error_test.go` | 174 | TC-E-040 … TC-E-044 (5 tests). |
| `test/e2e/recovery_test.go` | 150 | TC-E-050, TC-E-051 (2 tests — non-parallel). |
| `test/e2e/multiaccess_test.go` | 227 | TC-E-060, TC-E-061, TC-E-062 (3 tests). |

### 2.2 Testdata Inventory

| File | Purpose | Destination |
|------|---------|-------------|
| `test/e2e/testdata/kind-config.yaml` | kind cluster definition (1 control-plane + 1 worker) | Move to `test/chainsaw/kind-config.yaml` |
| `test/e2e/testdata/versitygw-deployment.yaml` | VersityGW Namespace + Secret + Deployment + Service | Move to `test/chainsaw/bootstrap/versitygw.yaml` |
| `test/e2e/testdata/bucketclass.yaml` | `versitygw-e2e` BucketClass (Delete) | Move to `test/chainsaw/bootstrap/bucketclass-default.yaml` |
| `test/e2e/testdata/bucketclass-retain.yaml` | `versitygw-e2e-retain` BucketClass (Retain) | Move to `test/chainsaw/bootstrap/bucketclass-retain.yaml` |
| `test/e2e/testdata/bucketclass-invalid.yaml` | `versitygw-e2e-invalid` BucketClass (bad params) | Move to `test/chainsaw/bootstrap/bucketclass-invalid.yaml` |
| `test/e2e/testdata/bucketaccessclass.yaml` | `versitygw-e2e` BucketAccessClass (Key) | Move to `test/chainsaw/bootstrap/bucketaccessclass-default.yaml` |
| `test/e2e/testdata/bucketaccessclass-iam.yaml` | `versitygw-e2e-iam` BucketAccessClass (IAM) | Move to `test/chainsaw/bootstrap/bucketaccessclass-iam.yaml` |

### 2.3 Infrastructure Dependencies

- `docker` (via `DOCKER_HOST=tcp://localhost:2375` per CLAUDE.md / user environment).
- `kind` v0.x for local cluster.
- `kubectl`, `helm`, `curl`.
- `chainsaw` CLI (NEW dependency).
- `jq` — needed inside Chainsaw verifier containers (will be bundled in our verifier image).

### 2.4 Known Issues We Inherit

1. **COSI v0.2.2 race condition**: Concurrent BucketClaim creations occasionally produce inconsistent `status` (e.g., `bucketReady=true` but `bucketName` empty). Observed today with Go E2E. Chainsaw `assert` with both fields in a single expression makes this a transient rather than permanent failure.
2. **COSI v0.2.2 CRDs lack `api-approved.kubernetes.io` annotation**: Kubernetes ≥1.35 rejects them. Current Go E2E injects this annotation at apply time via `curl | sed-like replace | kubectl apply -f -`. Plan must preserve this injection logic.
3. **Cluster-scoped resource conflicts in parallel**: Multiple tests applying the same BucketClass would race. Current Go E2E solves this by applying once in TestMain Setup. Plan must apply cluster-scoped resources in the bootstrap phase before Chainsaw tests run.
4. **BucketInfo format peculiarity**: Secret data has `BucketInfo` JSON with credentials at `spec.secretS3` (not `spec.s3.secretS3`). Verifier image must parse this path.
5. **Port-forward limitation**: In-cluster `spec.secretS3.endpoint` points to `versitygw.versitygw-system.svc.cluster.local:7070`. Verifier Pods run in-cluster, so they can use this endpoint directly (advantage over Go E2E host-side port-forward).

### 2.5 Environmental Assumptions

- `DOCKER_HOST=tcp://localhost:2375` — explicit for all Docker and kind operations.
- kind cluster version: whatever default `kind v0.x` uses (Kubernetes 1.34 or similar). Verified via testdata config (no explicit node image pin currently).
- Chainsaw binary installed via `curl -L` or `go install github.com/kyverno/chainsaw@vX.Y.Z`.
- Test runner has sufficient resources: 2 vCPU, 4GB RAM minimum (same as current).

---

## 3. Target Structure

```
test/
├── chainsaw/
│   ├── README.md                          # How to run locally and in CI
│   ├── kind-config.yaml                   # Cluster definition (moved from test/e2e/testdata)
│   ├── values.yaml                        # Chainsaw bindings: driverName, endpoints, etc.
│   ├── chainsaw-config.yaml               # Chainsaw execution config (parallelism, timeouts, reports)
│   ├── bootstrap/                         # One-time cluster setup manifests
│   │   ├── cosi-controller-install.sh     # Idempotent kubectl apply of CRDs + controller
│   │   ├── versitygw.yaml                 # VersityGW Namespace + Secret + Deployment + Service
│   │   ├── bucketclass-default.yaml
│   │   ├── bucketclass-retain.yaml
│   │   ├── bucketclass-invalid.yaml
│   │   ├── bucketaccessclass-default.yaml
│   │   └── bucketaccessclass-iam.yaml
│   ├── verifier/                          # Custom Pod verifier image
│   │   ├── Dockerfile                     # FROM amazon/aws-cli + jq
│   │   └── entrypoint.sh                  # Parses /conf/BucketInfo, runs parameterized S3 op
│   ├── setup.sh                           # Orchestrates: kind create + COSI + VersityGW + driver Helm + verifier image build/load
│   ├── teardown.sh                        # kind delete cluster
│   └── tests/
│       ├── tc-e-001-bucketclass/          # TC-E-001
│       │   └── chainsaw-test.yaml
│       ├── tc-e-002-bucketclaim/          # TC-E-002
│       │   └── chainsaw-test.yaml
│       ├── tc-e-003-bucketclaim-status/   # TC-E-003
│       │   └── chainsaw-test.yaml
│       ├── tc-e-004-bucketaccessclass/    # TC-E-004
│       │   └── chainsaw-test.yaml
│       ├── tc-e-005-bucketaccess-creates-secret/
│       ├── tc-e-006-bucketaccess-status/
│       ├── tc-e-007-delete-bucketaccess-removes-secret/
│       ├── tc-e-008-delete-bucketaccess-revokes-access/
│       ├── tc-e-009-delete-bucketclaim-removes-bucket/
│       ├── tc-e-010-full-lifecycle/
│       ├── tc-e-020-secret-namespace/
│       ├── tc-e-021-secret-bucketinfo-endpoint/
│       ├── tc-e-022-secret-credentials/
│       ├── tc-e-023-credentials-connect/
│       ├── tc-e-030-pod-putobject/
│       ├── tc-e-031-pod-getobject/
│       ├── tc-e-032-pod-listobjects/
│       ├── tc-e-040-bucketclaim-nonexistent-class/
│       ├── tc-e-041-bucketaccess-iam-auth/
│       ├── tc-e-042-bucketaccess-nonexistent-claim/
│       ├── tc-e-043-invalid-driver-parameters/
│       ├── tc-e-044-delete-bucketclaim-while-access-exists/
│       ├── tc-e-050-driver-restart/
│       ├── tc-e-051-cosi-controller-restart/
│       ├── tc-e-060-multiple-bucketaccess/
│       ├── tc-e-061-delete-one-bucketaccess/
│       └── tc-e-062-retain-policy/
└── (test/e2e/ removed entirely)
```

### 3.1 Verifier Image (`test/chainsaw/verifier`)

A single container image `versitygw-cosi-verifier:e2e` used for in-cluster S3 verification.

**Base image:** `amazon/aws-cli:latest` + `jq` installed via apk/yum (aws-cli base is amazonlinux).

**Image responsibility:** Given a mounted `/conf/BucketInfo` Secret, execute a parameterized S3 action and exit 0/1.

**Entrypoint interface:**
```
entrypoint.sh <op> [args...]

Operations:
  head-bucket                               # HeadBucket on this BucketInfo's bucket
  put-object <key> <data>                   # PutObject to this BucketInfo's bucket
  get-object <key> <expected-content>       # GetObject and verify content equals expected
  list-objects <expected-key> [expected-key ...]  # ListObjectsV2 and verify contains expected keys
  expect-denied <op> [args...]              # Runs any op and inverts success (expects 403/failure)
```

Credentials/endpoint/bucket read from `/conf/BucketInfo` JSON.

**Image life-cycle:** Built once in `setup.sh`, `kind load docker-image` into the cluster, referenced by tests as `image: versitygw-cosi-verifier:e2e`, `imagePullPolicy: Never`.

---

## 4. Test Migration Matrix

Every test case gets one directory `tc-e-XXX-<slug>/chainsaw-test.yaml`. The following matrix specifies the Chainsaw translation for each.

### 4.1 Lifecycle (TC-E-001 to TC-E-010)

| TC | Go Test | Chainsaw Strategy |
|----|---------|-------------------|
| TC-E-001 | TestBucketClassCreation | `apply` bootstrap BucketClass (no-op since already applied). `assert` it exists. |
| TC-E-002 | TestBucketClaimCreatesBucket | `apply` BucketClaim → `assert` `status.bucketReady: true` AND `status.bucketName: (exists)` AND Bucket CRD exists with matching name. |
| TC-E-003 | TestBucketClaimStatusReflectsBucketID | Subset of TC-E-002: `apply` BucketClaim → `assert` `status.bucketName` is non-empty string. |
| TC-E-004 | TestBucketAccessClassCreation | `assert` bootstrap BucketAccessClass exists. |
| TC-E-005 | TestBucketAccessCreatesSecret | `apply` BucketClaim → `assert` Ready → `apply` BucketAccess → `assert` Secret exists with `data.BucketInfo` present. |
| TC-E-006 | TestBucketAccessStatusReflectsAccountID | Same as TC-E-005 + assert Secret's BucketInfo has non-empty `spec.secretS3.accessKeyID`. |
| TC-E-007 | TestDeleteBucketAccessRemovesSecret | Create BucketClaim + BucketAccess → extract accessKeyID from Secret via an in-cluster `extract` Job that writes to a ConfigMap → delete BucketAccess → `error` assert Secret is gone → admin-mode verifier Job `list-users` (NEW op) → assert the extracted accessKeyID is NOT in the returned list. See Section 5.3. |
| TC-E-008 | TestDeleteBucketAccessRevokesAccess | Create full stack → verifier-before Job: `put-object test-key test-value` (succeed) → delete BucketAccess → verifier-after Job: `expect-403 put-object denied-key x` using OLD credentials → assert Job succeeds (i.e. denied). **Must be `expect-403` not generic `expect-denied`** to match Go's semantic. |
| TC-E-009 | TestDeleteBucketClaimRemovesBucket | Create BucketClaim → delete → `error` assert BucketClaim is gone → admin-mode verifier Job `head-bucket-not-found` (NEW op with explicit NoSuchBucket expectation) on bucket name. |
| TC-E-010 | TestFullLifecycle | Compose sequence of all above steps in one test. |

**Key design decision**: For TC-E-002 and TC-E-003, we use a single `assert` that requires BOTH `bucketReady: true` and `bucketName` to be set. This directly addresses the observed race condition.

### 4.2 Secret Validation (TC-E-020 to TC-E-023)

| TC | Strategy |
|----|----------|
| TC-E-020 | Create BucketAccess in namespace `ns` with `credentialsSecretName: creds`. Assert `Secret/creds` exists in `ns`. |
| TC-E-021 | Create BucketAccess → assert Secret's `data.BucketInfo` decoded contains `spec.secretS3.endpoint` non-empty and `spec.bucketName` non-empty. Use Chainsaw `jmespath` expression or a tiny Job that decodes and asserts. |
| TC-E-022 | Same but assert `spec.secretS3.accessKeyID` and `spec.secretS3.accessSecretKey` non-empty. |
| TC-E-023 | Create BucketAccess → Job with verifier image, op `head-bucket`, mount Secret. Assert Job success. |

**Note on JSON validation inside Chainsaw**: upstream COSI uses a Python Job with `check-jsonschema`. We follow the same pattern. Alternative: use Chainsaw's `(assert ...)` expressions on the Secret data if decoding base64 is feasible — test this during implementation.

### 4.3 Pod Consumption (TC-E-030 to TC-E-032)

**Major improvement**: Current Go tests simulate Pods by using port-forwarded S3 clients on the host. Chainsaw version uses REAL in-cluster Pods, which is more faithful to the spec wording "Pod can ... using Secret credentials".

| TC | Strategy |
|----|----------|
| TC-E-030 | Create BucketClaim + BucketAccess → verifier Pod: `entrypoint.sh put-object test-key test-value` → assert Pod phase=Succeeded. Then an **admin-mode** verifier Job `head-object test-key` (NEW op) to confirm object exists. |
| TC-E-031 | Setup: admin-mode verifier Job pre-writes object (`put-object`) → verifier Pod `get-object test-key expected-value` → assert Succeeded. |
| TC-E-032 | Setup: admin-mode verifier Job writes 3 objects → verifier Pod `list-objects obj-a obj-b obj-c` → assert Succeeded. |

### 4.4 Error Scenarios (TC-E-040 to TC-E-044)

| TC | Strategy |
|----|----------|
| TC-E-040 | Create BucketClaim referring to non-existent BucketClass → `error` assert BucketClaim.status.bucketReady is NEVER true within 30s (use Chainsaw `error` step or `assert` with negation expression). |
| TC-E-041 | Create BucketAccess with `versitygw-e2e-iam` class → `error` assert `accessGranted: true` never occurs; assert Secret is never created. |
| TC-E-042 | Create BucketAccess referring to non-existent BucketClaim → `error` assert never ready. |
| TC-E-043 | Create BucketClaim referring to `versitygw-e2e-invalid` class → `error` assert never ready (because driver rejects unknown parameters with INVALID_ARGUMENT). |
| TC-E-044 | Create BucketClaim + BucketAccess → delete BucketClaim (while BucketAccess still exists) → delete BucketAccess → assert Secret gone, BucketClaim gone. Verifies no orphaned state. |

### 4.5 Recovery (TC-E-050, TC-E-051)

**Key constraint**: Recovery tests restart shared infrastructure and MUST NOT run concurrently with any other E2E test.

**Baseline design (not fallback)**: Run recovery tests in a **separate Chainsaw invocation**, after the parallel suite completes:

```
make test-e2e              # runs test/chainsaw/tests/ (parallel, 25 tests, excludes recovery)
make test-e2e-recovery     # runs test/chainsaw/recovery/ (serial, 2 tests)
make test-e2e-all          # chains the two in order
```

This eliminates reliance on Chainsaw's `serial: true` feature (which may or may not exist in v1alpha2; see Appendix C). If a future Chainsaw version adds verified serial scheduling, the two invocations could collapse into one.

| TC | Strategy |
|----|----------|
| TC-E-050 | Create BucketClaim + BucketAccess → verifier Pod `put-object pre-restart` (Succeeded) → `script` step: `kubectl rollout restart deployment/<driver> ; kubectl rollout status ...` → assert existing resources still Ready → verifier Pod `put-object post-restart` → Create NEW BucketClaim/Access, assert Ready. |
| TC-E-051 | Same as 050 but restart the COSI controller. **Namespace/deployment pair must be discovered at runtime**, matching current Go logic that tries both `objectstorage-provisioner-system/objectstorage-provisioner-controller-manager` AND `container-object-storage-system/container-object-storage-controller` (see `test/e2e/recovery_test.go:38-60`). The recovery test's `script` step MUST probe both before restarting. |

### 4.6 Multi-Access (TC-E-060 to TC-E-062)

| TC | Strategy |
|----|----------|
| TC-E-060 | Create BucketClaim → Create 2 BucketAccesses (`a`, `b`) → assert both Secrets exist with distinct accessKeyIDs → 4-Job cross-verification matching Go parity (`test/e2e/multiaccess_test.go:51-89`): **(1)** Job `a-put-x` writes `written-by-a` via creds-a (Succeeded). **(2)** Job `b-put-y` writes `written-by-b` via creds-b (Succeeded). **(3)** Job `b-read-x` reads `written-by-a` via creds-b (Succeeded — cross-read B→A). **(4)** Job `a-read-y` reads `written-by-b` via creds-a (Succeeded — cross-read A→B). All four must succeed; any omitted Job weakens parity vs Go. |
| TC-E-061 | Same setup → delete BucketAccess `a` → assert Secret `creds-a` gone → verifier Job using `creds-a` credentials `expect-403 put-object` (use the saved Secret data mounted into the Job, since the live Secret is deleted — see Section 5.4 for the credential-snapshot pattern) → verifier Job using `creds-b` `put-object` (Succeeded) → assert BucketAccess `b` still Ready. |
| TC-E-062 | Create BucketClaim with `versitygw-e2e-retain` class → wait Ready → record bucket name via Chainsaw binding from `status.bucketName` → delete BucketClaim → assert BucketClaim gone → admin-mode verifier Job `head-bucket-by-name <name>` (Succeeded, because Retain kept it) → cleanup bucket via admin-mode verifier Job `delete-bucket-by-name <name>` (NEW op). |

### 4.7 Migration Matrix Summary Counts

- **Pure declarative (apply + assert)**: 10 tests (TC-E-001, 002, 003, 004, 005, 006, 020, 040, 041, 042)
- **Declarative + single Job verifier**: 8 tests (TC-E-009, 021, 022, 023, 030, 031, 032, 043, 062)
- **Multiple Jobs + sequential flow**: 6 tests (TC-E-007, 008, 010, 044, 060, 061)
- **Script + Pod/Job combo**: 2 tests (TC-E-050, 051)

Total: 26 tests. TC-E-044 absorbs into multi-Job category, TC-E-043 into single-Job (uses bootstrap BucketClass). Adjusted: 10 + 9 + 5 + 2 = 26 — off by one; TC-E-044 listed in section 4.4 but moved by category to multi-Job. Final total 27 matches.

---

## 5. Verifier Image Specification

### 5.1 Dockerfile

```dockerfile
FROM amazon/aws-cli:latest
RUN yum install -y jq && yum clean all
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
COPY admin.sh /usr/local/bin/admin.sh
RUN chmod +x /usr/local/bin/entrypoint.sh /usr/local/bin/admin.sh
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
```

**Image size caveat**: `amazon/aws-cli:latest` is ~350MB. Acceptable for a single-node kind cluster on a dev box, but measure disk footprint (`docker system df`) after `kind load`. If disk pressure appears in CI, switch to a minimal Go binary that uses `aws-sdk-go-v2` (~30MB static image). **Phase 2 validation gate MUST measure this.**

### 5.2 entrypoint.sh — user-credentials mode

Reads credentials from a mounted BucketInfo Secret. Supports ops that a BucketAccess consumer would perform.

```bash
#!/bin/sh
set -euo pipefail
BI=${BUCKETINFO_PATH:-/conf/BucketInfo}

ACCESS=$(jq -r '.spec.secretS3.accessKeyID' "$BI")
SECRET=$(jq -r '.spec.secretS3.accessSecretKey' "$BI")
ENDPOINT=$(jq -r '.spec.secretS3.endpoint' "$BI")
BUCKET=$(jq -r '.spec.bucketName' "$BI")

export AWS_ACCESS_KEY_ID="$ACCESS"
export AWS_SECRET_ACCESS_KEY="$SECRET"
export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-us-east-1}"

OP=$1
shift

run_with_status_capture() {
  # Runs aws-cli, captures stderr to detect HTTP status codes.
  set +e
  ERR=$(aws "$@" 2>&1 >/tmp/aws.out)
  RC=$?
  set -e
  echo "$ERR" > /tmp/aws.err
  return $RC
}

case "$OP" in
  head-bucket)
    aws s3api head-bucket --endpoint-url "$ENDPOINT" --bucket "$BUCKET" ;;
  put-object)
    KEY=$1; DATA=$2
    echo -n "$DATA" | aws s3api put-object --endpoint-url "$ENDPOINT" --bucket "$BUCKET" --key "$KEY" --body /dev/stdin ;;
  get-object)
    KEY=$1; EXPECTED=$2
    aws s3api get-object --endpoint-url "$ENDPOINT" --bucket "$BUCKET" --key "$KEY" /tmp/out
    ACTUAL=$(cat /tmp/out)
    [ "$ACTUAL" = "$EXPECTED" ] || { echo "mismatch: got=$ACTUAL want=$EXPECTED"; exit 2; } ;;
  list-objects)
    OUT=$(aws s3api list-objects-v2 --endpoint-url "$ENDPOINT" --bucket "$BUCKET" --query 'Contents[].Key' --output text)
    for key in "$@"; do
      echo "$OUT" | grep -qw "$key" || { echo "missing key: $key"; exit 3; }
    done ;;
  expect-403)
    # Runs any op, expects failure with HTTP 403 / Access Denied.
    INNER=$1; shift
    set +e
    ERR=$(/usr/local/bin/entrypoint.sh "$INNER" "$@" 2>&1)
    RC=$?
    set -e
    if [ $RC -eq 0 ]; then echo "expected 403 but succeeded"; exit 4; fi
    echo "$ERR" | grep -qE 'AccessDenied|403|Forbidden' || { echo "expected 403, got: $ERR"; exit 5; }
    ;;
  expect-nosuchbucket)
    INNER=$1; shift
    set +e
    ERR=$(/usr/local/bin/entrypoint.sh "$INNER" "$@" 2>&1)
    RC=$?
    set -e
    if [ $RC -eq 0 ]; then echo "expected NoSuchBucket but succeeded"; exit 4; fi
    echo "$ERR" | grep -qE 'NoSuchBucket|404|Not Found' || { echo "expected 404, got: $ERR"; exit 5; }
    ;;
  extract-access-key)
    # Writes the accessKeyID to a ConfigMap-friendly file at /output/access-key.
    mkdir -p /output
    echo -n "$ACCESS" > /output/access-key ;;
  *)
    echo "unknown user-mode op: $OP" ; exit 64 ;;
esac
```

### 5.3 admin.sh — admin-credentials mode

Reads root credentials from env vars (sourced from `versitygw-root-credentials` Secret via `envFrom`). Supports ops that require admin privileges.

```bash
#!/bin/sh
set -euo pipefail

# Required env vars from Secret:
# - ROOT_ACCESS_KEY  (key: rootAccessKeyId)
# - ROOT_SECRET_KEY  (key: rootSecretAccessKey)
# Required from ConfigMap/args:
# - S3_ENDPOINT      (default: http://versitygw.versitygw-system.svc.cluster.local:7070)
# - ADMIN_ENDPOINT   (default: http://versitygw.versitygw-system.svc.cluster.local:7071)

export AWS_ACCESS_KEY_ID="$ROOT_ACCESS_KEY"
export AWS_SECRET_ACCESS_KEY="$ROOT_SECRET_KEY"
export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-us-east-1}"
S3=${S3_ENDPOINT:-http://versitygw.versitygw-system.svc.cluster.local:7070}

OP=$1
shift

case "$OP" in
  head-bucket-by-name)
    aws s3api head-bucket --endpoint-url "$S3" --bucket "$1" ;;
  head-bucket-not-found)
    set +e
    aws s3api head-bucket --endpoint-url "$S3" --bucket "$1" 2>/tmp/err
    RC=$?
    set -e
    if [ $RC -eq 0 ]; then echo "expected not-found but succeeded"; exit 4; fi
    grep -qE 'NoSuchBucket|404|Not Found' /tmp/err || { echo "unexpected error"; cat /tmp/err; exit 5; } ;;
  head-object)
    # Args: <bucket> <key>
    aws s3api head-object --endpoint-url "$S3" --bucket "$1" --key "$2" ;;
  put-object)
    # Args: <bucket> <key> <data>
    echo -n "$3" | aws s3api put-object --endpoint-url "$S3" --bucket "$1" --key "$2" --body /dev/stdin ;;
  delete-bucket-by-name)
    aws s3api delete-bucket --endpoint-url "$S3" --bucket "$1" ;;
  list-users)
    # VersityGW admin API list-users is PATCH with SigV4 on ADMIN_ENDPOINT.
    # aws-cli cannot directly call arbitrary PATCH endpoints; use curl + aws-sigv4.
    ADMIN=${ADMIN_ENDPOINT:-http://versitygw.versitygw-system.svc.cluster.local:7071}
    curl -sS -f -X PATCH "$ADMIN/list-users" \
      --aws-sigv4 "aws:amz:$AWS_DEFAULT_REGION:s3" \
      --user "$AWS_ACCESS_KEY_ID:$AWS_SECRET_ACCESS_KEY" ;;
  assert-user-absent)
    # Args: <expected-absent-access-key>
    ADMIN=${ADMIN_ENDPOINT:-http://versitygw.versitygw-system.svc.cluster.local:7071}
    OUT=$(curl -sS -f -X PATCH "$ADMIN/list-users" \
      --aws-sigv4 "aws:amz:$AWS_DEFAULT_REGION:s3" \
      --user "$AWS_ACCESS_KEY_ID:$AWS_SECRET_ACCESS_KEY")
    echo "$OUT" | grep -q "$1" && { echo "user $1 still present"; exit 6; } || exit 0 ;;
  *)
    echo "unknown admin-mode op: $OP" ; exit 64 ;;
esac
```

### 5.4 Credential-snapshot pattern (for TC-E-061)

TC-E-061 needs to verify that **deleted** BucketAccess credentials are actually denied. After the Secret is gone, we can't mount it. Pattern:

1. Before deleting the BucketAccess, an `extract` Job mounts the Secret and writes accessKeyID/secretKey/endpoint/bucket values to a new Secret `creds-a-snapshot` in the test's namespace.
2. Delete the BucketAccess.
3. The verifier Job mounts `creds-a-snapshot` (not the deleted one) and runs `expect-403 put-object ...`.

This mirrors how Go E2E saves `s3Client` reference via Go closure before revocation (`test/e2e/lifecycle_test.go:270-280`).

### 5.5 Op matrix (parity check)

| Op | Mode | Used by |
|----|------|---------|
| `head-bucket` | user | TC-E-023, TC-E-062 (via admin variant) |
| `put-object` | user | TC-E-008 (before), TC-E-030, TC-E-060, TC-E-061 |
| `get-object` | user | TC-E-031 |
| `list-objects` | user | TC-E-032 |
| `expect-403` | user | TC-E-008 (after), TC-E-061 |
| `expect-nosuchbucket` | user | (not currently used; reserved) |
| `extract-access-key` | user | TC-E-007 (helper) |
| `head-bucket-by-name` | admin | TC-E-062 |
| `head-bucket-not-found` | admin | TC-E-009 |
| `head-object` | admin | TC-E-030 |
| `put-object` | admin | TC-E-031 setup, TC-E-032 setup |
| `delete-bucket-by-name` | admin | TC-E-062 cleanup |
| `list-users` | admin | TC-E-007 (for debugging) |
| `assert-user-absent` | admin | TC-E-007 |

---

## 6. Infrastructure Changes

### 6.1 setup.sh orchestrator

```bash
#!/bin/bash
set -euo pipefail

CLUSTER=${KIND_CLUSTER:-versitygw-cosi-e2e}
DOCKER_HOST=${DOCKER_HOST:-tcp://localhost:2375}
export DOCKER_HOST

# 1. Create kind cluster
kind create cluster --name "$CLUSTER" --config test/chainsaw/kind-config.yaml

# 2. Install COSI controller (v0.2.2, with api-approved annotation injection)
./test/chainsaw/bootstrap/cosi-controller-install.sh

# 3. Deploy VersityGW
kubectl apply -f test/chainsaw/bootstrap/versitygw.yaml
kubectl wait --for=condition=available --timeout=120s deployment/versitygw -n versitygw-system

# 4. Build + load driver image
docker build -t versitygw-cosi-driver:e2e .
kind load docker-image versitygw-cosi-driver:e2e --name "$CLUSTER"

# 5. Build + load verifier image
docker build -t versitygw-cosi-verifier:e2e test/chainsaw/verifier/
kind load docker-image versitygw-cosi-verifier:e2e --name "$CLUSTER"

# 6. Copy root credentials Secret to driver namespace
kubectl create namespace versitygw-cosi-driver-system --dry-run=client -o yaml | kubectl apply -f -
kubectl get secret versitygw-root-credentials -n versitygw-system -o yaml \
  | sed 's/namespace: versitygw-system/namespace: versitygw-cosi-driver-system/' \
  | kubectl apply -f -

# 7. Install driver via Helm
helm install versitygw-cosi-driver-e2e deploy/helm/versitygw-cosi-driver \
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

# 8. Apply shared cluster-scoped resources
kubectl apply -f test/chainsaw/bootstrap/bucketclass-default.yaml
kubectl apply -f test/chainsaw/bootstrap/bucketclass-retain.yaml
kubectl apply -f test/chainsaw/bootstrap/bucketclass-invalid.yaml
kubectl apply -f test/chainsaw/bootstrap/bucketaccessclass-default.yaml
kubectl apply -f test/chainsaw/bootstrap/bucketaccessclass-iam.yaml

echo "Cluster $CLUSTER ready for Chainsaw tests."
```

### 6.2 cosi-controller-install.sh

Replicates the current Go `installCOSIController` logic: download v0.2.2 CRDs via curl, inject `api-approved.kubernetes.io` annotation, apply. Apply controller via `kubectl apply -k <upstream-ref>`.

### 6.3 teardown.sh

```bash
#!/bin/bash
kind delete cluster --name "${KIND_CLUSTER:-versitygw-cosi-e2e}"
```

### 6.4 chainsaw-config.yaml

```yaml
apiVersion: chainsaw.kyverno.io/v1alpha2
kind: Configuration
metadata:
  name: versitygw-cosi-driver-e2e
spec:
  parallel: 8
  timeouts:
    apply: 30s
    assert: 3m
    error: 30s
    delete: 30s
    cleanup: 60s
    exec: 60s
  namespace: ""    # Each test gets its own ephemeral namespace
  fullName: true
  # Serial tests: include recovery TCs here or tag via test metadata
```

### 6.5 values.yaml (shared bindings)

```yaml
driverName: versitygw.cosi.dev
bucketClassDefault: versitygw-e2e
bucketClassRetain: versitygw-e2e-retain
bucketClassInvalid: versitygw-e2e-invalid
bucketAccessClassDefault: versitygw-e2e
bucketAccessClassIAM: versitygw-e2e-iam
verifierImage: versitygw-cosi-verifier:e2e
rootCredsSecret: versitygw-root-credentials
versitygwNamespace: versitygw-system
driverNamespace: versitygw-cosi-driver-system
driverDeployment: versitygw-cosi-driver-e2e
cosiControllerNamespace: container-object-storage-system
cosiControllerDeployment: container-object-storage-controller
```

### 6.6 Makefile changes

```makefile
.PHONY: build test integration-test test-e2e test-e2e-recovery test-e2e-setup test-e2e-teardown test-e2e-all docker-build clean lint lint-fix

# ... existing targets ...

# Note: DOCKER_HOST default is intentionally empty; CI typically uses the default
# unix socket, while local dev on Arch/Manjaro requires tcp://localhost:2375.
# Export DOCKER_HOST in the environment (or prefix the command) as needed.

test-e2e-setup:
	./test/chainsaw/setup.sh

test-e2e:
	chainsaw test test/chainsaw/tests \
	    --config test/chainsaw/chainsaw-config.yaml \
	    --values test/chainsaw/values.yaml

test-e2e-recovery:
	chainsaw test test/chainsaw/recovery \
	    --config test/chainsaw/chainsaw-config.yaml \
	    --values test/chainsaw/values.yaml \
	    --parallel 1

test-e2e-teardown:
	./test/chainsaw/teardown.sh

# test-e2e-all uses || to force teardown even on failure. This preserves the
# "no cluster leaks across runs" guarantee regardless of test outcome.
test-e2e-all:
	$(MAKE) test-e2e-setup
	{ $(MAKE) test-e2e && $(MAKE) test-e2e-recovery ; } ; rc=$$? ; $(MAKE) test-e2e-teardown ; exit $$rc

# Diagnostic target: keeps the cluster for inspection after failure.
test-e2e-keep:
	$(MAKE) test-e2e-setup
	$(MAKE) test-e2e ; true
	@echo "Cluster retained for inspection. Run 'make test-e2e-teardown' when done."
```

### 6.7 CI changes (`.github/workflows/ci.yaml`)

The existing workflow's trigger list (`push` to master, `pull_request` to master) must be extended with `workflow_dispatch` and support PR label gating. The `e2e` job must set up Go (required for `go install chainsaw`), set DOCKER_HOST appropriately for GitHub runners (default unix socket, NOT tcp:2375), and explicitly install kind/helm/chainsaw.

```yaml
# Addition to `on:` at top of file
on:
  push:
    branches: [master]
  pull_request:
    branches: [master]
    types: [opened, synchronize, reopened, labeled]
  workflow_dispatch:

# New job, appended at bottom
  e2e:
    runs-on: ubuntu-latest
    if: >-
      github.event_name == 'workflow_dispatch' ||
      (github.event_name == 'pull_request' &&
       contains(github.event.pull_request.labels.*.name, 'run-e2e'))
    timeout-minutes: 30
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
      - uses: azure/setup-helm@v4
      - name: Install kind
        uses: helm/kind-action@v1.10.0
        with:
          install_only: true
      - name: Install Chainsaw
        # Version MUST be re-validated during Phase 1. The value below is
        # a placeholder that Phase 1 must replace with a verified version
        # after running `go install github.com/kyverno/chainsaw@latest` and
        # confirming that the test YAML `v1alpha2` schema is supported.
        run: curl -fsSL https://github.com/kyverno/chainsaw/releases/download/v0.2.14/chainsaw_linux_amd64.tar.gz \
  | tar -xz -C $HOME/.local/bin chainsaw
      - name: Show tool versions
        run: |
          docker version
          kind version
          kubectl version --client
          helm version
          chainsaw version
      - name: Bootstrap cluster
        run: make test-e2e-setup
        # Do NOT set DOCKER_HOST=tcp://localhost:2375 on GitHub runners;
        # they use the default unix socket. Local dev users override via env.
      - name: Run Chainsaw tests (parallel)
        run: make test-e2e
      - name: Run Chainsaw tests (recovery, serial)
        run: make test-e2e-recovery
      - name: Dump logs on failure
        if: failure()
        run: |
          kubectl get all -A || true
          kubectl logs -n container-object-storage-system deployment/container-object-storage-controller --tail=200 || true
          kubectl logs -n versitygw-cosi-driver-system deployment/versitygw-cosi-driver-e2e -c driver --tail=200 || true
      - name: Teardown
        if: always()
        run: make test-e2e-teardown
```

**Trigger strategy**: initial rollout as manual dispatch + PR label `run-e2e`. Once stable for N PRs (say 5 consecutive green), gate on pushes to master.

**Chainsaw version pin**: `v0.2.14` (verified 2026-04-18; see Appendix C). Installed via precompiled Linux amd64 tarball from GitHub releases. `go install` does not work against Go 1.26 due to a `testing.testDeps` interface mismatch.

---

## 7. Documentation Updates

### 7.0 Project-wide search for stale `test/e2e/` references (MUST grep before Phase 7)

Before deleting `test/e2e/`, grep the entire tree for references (not just known doc files):

```bash
git grep -n 'test/e2e' -- ':!docs/plans/chainsaw-migration-plan.md'
git grep -n 'sigs.k8s.io/e2e-framework' -- ':!docs/plans/chainsaw-migration-plan.md'
```

Known locations to audit and update (not exhaustive — confirmed by grep during Phase 7):

- `README.md` (root) — currently references `kubectl create -k ...v0.2.2` install path; if docs are touched, cross-reference the annotation-injection workaround. Verify E2E-related mentions.
- `CONTRIBUTING.md` — Go-based E2E run instructions must be rewritten.
- `docs/tests/README.md` (lines ~97-109) — frames E2E under `//go:build e2e` convention; rewrite.
- `deploy/helm/versitygw-cosi-driver/README.md` (lines ~21-23) — Helm install instructions; verify.
- `deploy/helm/versitygw-cosi-driver/templates/NOTES.txt` (lines ~3-7) — also carries CRD install path.
- `.golangci.yml` — current state already has no `test/e2e/` exception (confirmed by earlier commit); verify.
- `go.mod` AND `go.sum` — both must be regenerated after removing `sigs.k8s.io/e2e-framework`.
- `go.work` / `go.work.sum` (if present) — check.

### 7.1 `docs/tests/e2e-tests.md`

Full rewrite:

- **Purpose** section: preserved.
- **Architecture** section: updated to describe Chainsaw-based layout.
- **Test Cases** section: preserve all 27 TC definitions (Given/When/Then). Add a "Chainsaw mapping" sub-line for each pointing to `test/chainsaw/tests/tc-e-XXX-<slug>/chainsaw-test.yaml`.
- **Go Conventions** section: removed.
- **Chainsaw Conventions** section: added. Covers template bindings, verifier image usage, naming conventions.
- **How to Run** section: updated to `make test-e2e-all` and local chainsaw invocation.

### 7.2 `docs/tests/README.md`

- Update the "Go Implementation Notes" line under E2E to "Chainsaw YAML files under `test/chainsaw/tests/`."
- Update test pyramid diagram comments ("Chainsaw" label under E2E).

### 7.3 `CHANGELOG.md` (Unreleased)

```markdown
### Added
- E2E test suite migrated to Kyverno Chainsaw, aligning with upstream COSI conformance practice.
- Verifier image `versitygw-cosi-verifier:e2e` wrapping `amazon/aws-cli + jq` for in-cluster S3 assertions.
- `make test-e2e-setup`, `make test-e2e`, `make test-e2e-teardown`, `make test-e2e-all` targets.

### Removed
- Legacy Go-based E2E suite under `test/e2e/` (sigs.k8s.io/e2e-framework, testify). Replaced by Chainsaw YAML tests under `test/chainsaw/tests/`.
```

### 7.4 `CLAUDE.md`

Add gotcha:

```markdown
- E2E tests use Kyverno Chainsaw (not Go test framework). Run via `make test-e2e-all` or see `test/chainsaw/README.md`.
```

Remove any stale references to `test/e2e/`.

### 7.5 `.golangci.yml`

Already has no `test/e2e/` references after the recent commit. Verify during implementation.

### 7.6 `go.mod` / `go.sum`

Remove `sigs.k8s.io/e2e-framework` and transitive deps no longer used. Run `go mod tidy` to audit both `go.mod` AND `go.sum`.

### 7.7 `CONTRIBUTING.md`

Rewrite the E2E section with Chainsaw instructions. Match the Makefile targets defined in Section 6.6. Preserve any non-E2E content verbatim.

---

## 8. Phases & Deliverables

### Phase 0 — Preparation (est. 0.5 day)

- [ ] Commit current branch state (already done for non-E2E).
- [ ] Create feature branch `chainsaw-migration`.
- [ ] Document the migration plan (this file) and get user sign-off.
- [ ] Install Chainsaw locally, verify version compatibility.

### Phase 1 — Infrastructure Skeleton (est. 1 day)

- [ ] Create `test/chainsaw/` directory structure.
- [ ] Move testdata YAMLs to `test/chainsaw/bootstrap/`.
- [ ] Write `setup.sh`, `teardown.sh`, `cosi-controller-install.sh`.
- [ ] Write `chainsaw-config.yaml`, `values.yaml`, `kind-config.yaml`.
- [ ] Write `test/chainsaw/README.md`.
- [ ] Add Makefile targets.
- [ ] **Validation gate**: `make test-e2e-setup` brings up a working cluster; manually create a BucketClaim and verify Ready; `make test-e2e-teardown` cleans up.

### Phase 2 — Verifier Image (est. 0.5 day)

- [ ] Write `Dockerfile` + `entrypoint.sh`.
- [ ] Add admin-credentials variant (env var mode).
- [ ] Build and verify locally: `docker run --rm -v /tmp/bi:/conf versitygw-cosi-verifier:e2e head-bucket`.
- [ ] **Validation gate**: Image runs head-bucket against a live VersityGW instance.

### Phase 3 — Pilot Tests (est. 1 day)

Port 3 representative tests first to validate the approach:

- [ ] TC-E-001 (pure declarative)
- [ ] TC-E-023 (single Job verifier with user creds)
- [ ] TC-E-009 (deletion + admin Job verifier)

- [ ] **Validation gate**: `chainsaw test test/chainsaw/tests/tc-e-001-bucketclass test/chainsaw/tests/tc-e-023-credentials-connect test/chainsaw/tests/tc-e-009-delete-bucketclaim-removes-bucket` all pass in parallel and in isolation.

### Phase 4 — Bulk Migration (est. 2 days)

Port remaining tests in groups:

- [ ] Group A: Lifecycle TC-E-002, 003, 004, 005, 006.
- [ ] Group B: Delete/Revoke TC-E-007, 008, 010.
- [ ] Group C: Secret TC-E-020, 021, 022.
- [ ] Group D: Pod TC-E-030, 031, 032.
- [ ] Group E: Error TC-E-040, 041, 042, 043, 044.
- [ ] Group F: Multi-access TC-E-060, 061, 062.

Each group validated by `chainsaw test test/chainsaw/tests/<group-prefix>*` passing.

### Phase 5 — Serial Tests: Recovery (est. 0.5 day)

- [ ] TC-E-050 — driver restart. Declare `serial: true` or use Chainsaw's ordering mechanism.
- [ ] TC-E-051 — controller restart.

- [ ] **Validation gate**: Recovery tests run AFTER parallel tests; parallel tests pass unaffected.

### Phase 6 — Full Integration Run (est. 0.5 day)

- [ ] `make test-e2e-all` end-to-end, 3 consecutive runs, all 27 pass.
- [ ] Verify total runtime under 15 minutes.
- [ ] Verify CI job works (manual dispatch trigger).

### Phase 7 — Docs & Cleanup (est. 0.5 day)

- [ ] Update `docs/tests/e2e-tests.md` with Chainsaw mapping lines per TC.
- [ ] Update `docs/tests/README.md`.
- [ ] Update `CHANGELOG.md`.
- [ ] Update `CLAUDE.md`.
- [ ] Update `.golangci.yml` if needed.
- [ ] Remove `test/e2e/` directory entirely.
- [ ] Run `go mod tidy` to remove unused deps.
- [ ] Verify `go test ./...` (non-E2E) still passes.
- [ ] Verify `go vet ./...` (non-E2E) passes.

### Phase 8 — Review & Merge (est. 0.5 day)

- [ ] Create PR.
- [ ] Get user review.
- [ ] Address feedback.
- [ ] Merge.

**Total estimated effort: 8–10 days** (revised after Codex review)

Phases most likely to overrun, in order:
1. **Phase 2** (Verifier image): admin-mode `list-users` via SigV4-signed PATCH is not a trivial curl call — may require Go helper or a Python/boto3 variant. +1 day buffer.
2. **Phase 3** (Pilot): Race condition re-verification on a Chainsaw test is a new unknown. If it does not converge, investigation time. +1 day buffer.
3. **Phase 5** (Recovery): `script` step environment and `kubectl rollout` orchestration inside Chainsaw may need iteration. +0.5 day buffer.

Upper bound estimate: 10 days. If Phase 3 gate fails decisively, total could balloon or plan is abandoned (Section 10 rollback #4).

---

## 9. Risk Analysis

### 9.1 Critical Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Chainsaw cannot express a specific assertion pattern we need | Medium | High | Phase 3 pilot explicitly tests the most complex scenarios (TC-E-009 admin verification). If fundamental gap found, halt and reassess. |
| Chainsaw version pin `v0.2.14` | Resolved 2026-04-18 | — | Installed and verified. See Appendix C. |
| Race condition fix hypothesis is overstated | Medium | Medium | The Chainsaw `assert` atomicity only helps if the inconsistency is transient (controller eventually self-heals). From repo evidence we observed the inconsistency persisting beyond 3 min, meaning Chainsaw would also time out. **Phase 3 MUST reproduce the race against a Chainsaw test and confirm it converges**. If it does not, the migration still has value for other reasons (maintainability, upstream parity), but timeouts must be longer and this risk document must record that Chainsaw is NOT a race-condition fix. |
| `amazon/aws-cli:latest` image is ~350MB; multiple kind loads + multiple Pod starts may exhaust node disk/CPU | Medium | Medium | Phase 2 validation gate measures `docker system df` after `kind load` and observes Pod startup latency. If exceeded, swap to minimal Go verifier image (aws-sdk-go-v2 static binary, ~30MB). |
| TC-E-008 `expect-403` may catch unrelated errors (e.g., network timeout during cluster turbulence) and produce false PASS | Medium | High | `expect-403` entrypoint grep's for `AccessDenied|403|Forbidden` strings in stderr. If ambiguous, escalate to a Go helper that parses aws-sdk error codes. Acceptance test in Phase 3. |
| Serial ordering design depends on separate invocation (two `chainsaw test` calls) — Makefile orchestration must handle partial failure | Low | Medium | `test-e2e-all` target uses `; rc=$$? ; ... exit $$rc` pattern to force teardown. Explicit test in Phase 6. |
| CI runner runtime exceeds timeout (30 min) when running parallel + recovery sequentially | Medium | Medium | Measure in Phase 6 local. If >25 min, split CI into two jobs or parallelize more aggressively. `timeout-minutes: 30` set on job — monitor. |
| During Phase 1-6, both `test/e2e/` (old Go) and `test/chainsaw/` (new) exist. If run accidentally concurrently, they share kind cluster name and shared cluster-scoped resources → conflict | **High** | High | The Go `setup_test.go` uses kind cluster name `versitygw-cosi-e2e`. The Chainsaw `setup.sh` MUST use the SAME kind cluster name during migration so parallel invocation is impossible by construction, OR it MUST use a distinct cluster name (`versitygw-cosi-chainsaw`) and a CI/Makefile guard prevents running both. **Decision: use a distinct kind cluster name `versitygw-cosi-chainsaw` during migration; after Phase 7 when Go E2E is removed, rename to `versitygw-cosi-e2e`.** |

### 9.2 Moderate Risks

| Risk | Mitigation |
|------|------------|
| Cluster-scoped resource leaks between test runs | `teardown.sh` destroys the cluster fully. No leaks possible across runs. |
| Image rebuilds on every run slow CI | Cache aws-cli base layer; driver image changes only when source changes. |
| Documentation drift between plan and actual implementation | Keep plan as living doc under `docs/plans/`. Update during implementation. |
| BucketInfo format changes with COSI upstream | We're pinned to v0.2.2. If we upgrade, the verifier image parsing needs update. Document this dependency. |

### 9.3 Low Risks (noted but not actively mitigated)

- Chainsaw project deprecation (low likelihood, Kyverno is active).
- kind project deprecation (very low).
- Upstream COSI repo restructure (document pin: `v0.2.2`).

---

## 10. Rollback Plan

If migration fails at any phase:

1. The `test/e2e/` directory is preserved on master until Phase 7. At any point before Phase 7, we can abandon the branch and keep Go E2E.
2. During Phases 1-6, the `test/chainsaw/` directory is additive. **However, additive ≠ safe to run both concurrently.** The two suites share conceptually the same cluster resources if run on the same cluster. Mitigation: use a distinct kind cluster name (`versitygw-cosi-chainsaw`) per Section 9.1 risk, AND add a CI guard (`if` conditions) that prevents both `e2e` (Go — currently not in CI) and `e2e-chainsaw` (new) from running in the same invocation.
3. If Phase 7 has already begun and a critical issue surfaces post-merge, revert the merge commit. Go E2E code is preserved in git history.
4. Intermediate abandonment: if Phase 3 pilot fails to converge within 2 extra days, halt and present findings to user. Do not sunk-cost into migration.

---

## 11. Interaction with Existing Work

### 11.1 Non-E2E tests

- Unit, component, integration layers: **untouched**. Same CI, same Makefile targets.
- `internal/driver/provisioner.go` parameter rejection: **preserved**. TC-E-043 relies on it.

### 11.2 Pending work

- **Chainsaw conformance from upstream** (previously tracked in `project_chainsaw_todo.md`): this plan **does NOT subsume upstream conformance**. Our migrated suite uses custom verifier ops (`expect-403`, `head-bucket-not-found`, `extract-access-key`, admin `list-users`, `assert-user-absent`, `delete-bucket-by-name`) that are NOT in the upstream template. Therefore upstream conformance is a **separate, additional** artifact.

  **Follow-up**: after this migration completes, add a **verbatim copy** of `kubernetes-sigs/container-object-storage-interface/test/e2e/chainsaw-test.yaml` under `test/chainsaw/conformance/` with a separate `make test-e2e-conformance` target. This gives us the upstream conformance stamp alongside our richer functional tests. Tracked as a distinct follow-up, not part of this migration's acceptance criteria.

### 11.3 Commits

- Work on a single feature branch `chainsaw-migration` off current master.
- One big PR at the end. Do not interleave with unrelated changes.

---

## 12. Open Questions / Decisions Needed

1. **Chainsaw version pinning**: **v0.2.14** verified 2026-04-18, precompiled Linux amd64 binary (see Appendix C).
2. **Do we keep a "conformance" variant separately**: **Yes, separate.** Section 11.2 clarifies our custom verifier ops diverge from upstream. Upstream conformance is a follow-up under `test/chainsaw/conformance/`, not part of this migration's acceptance.
3. **Recovery tests in default run**: **No.** Separate `make test-e2e-recovery` invocation, baseline design (Section 4.5).
4. **Verifier image location**: Local build only initially. Rebuilt per CI run from `test/chainsaw/verifier/`. Can push to GHCR once stable (follow-up).
5. **Remove `sigs.k8s.io/e2e-framework` from go.mod**: Yes, after Phase 7 deletion of `test/e2e/`. Run `go mod tidy`.
6. **CI trigger for E2E**: Label-gated + manual dispatch initially (Section 6.7). Re-evaluate after 5 consecutive green runs.
7. **Kind cluster name during migration**: `versitygw-cosi-chainsaw` (distinct from Go E2E's `versitygw-cosi-e2e`). Rename to `versitygw-cosi-e2e` in Phase 7 after Go code is removed. See Section 9.1.
8. **Go verifier fallback trigger**: aws-cli image >400MB on disk OR Pod cold start >10s. Decided in Phase 2 validation gate (Section 5.1 note).

---

## 13. Test Mapping Preservation Checklist

For each TC, confirm the Chainsaw test asserts at least as much as the Go test. This section is a table that must be filled in during Phase 3-4 implementation, one row per TC:

| TC | Go assertions (summary) | Chainsaw assertions (summary) | Parity |
|----|-------------------------|-------------------------------|--------|
| TC-E-001 | BucketClass resource exists, no status errors | `assert` BucketClass exists | ✅ |
| TC-E-002 | `bucketReady=true` within 60s; Bucket CRD exists | `assert` BucketClaim `status.bucketReady: true` AND `status.bucketName: <non-empty>`; `assert` Bucket CRD with matching name | ✅ stricter |
| TC-E-003 | `status.bucketName` non-empty | Same — included in TC-E-002 atomic assert | ✅ |
| TC-E-004 | BucketAccessClass exists, no errors | `assert` exists | ✅ |
| TC-E-005 | BucketAccess ready, Secret exists with `BucketInfo` key | `apply` BucketAccess; `assert` BucketAccess `status.accessGranted: true`; `assert` Secret with `data.BucketInfo` present | ✅ |
| TC-E-006 | Secret's BucketInfo has non-empty accessKeyID (as proxy for account id) | Same | ✅ |
| TC-E-007 | After delete BucketAccess: Secret gone; VersityGW user absent | `error` Secret gone; Job using admin creds list-users shows accessKeyID absent | ✅ |
| TC-E-008 | Credentials from Secret can PutObject before delete, PutObject 403 after | 2 Jobs: before (succeed), after (expect-denied) | ✅ |
| TC-E-009 | After BucketClaim delete, bucket absent in VersityGW | `error` BucketClaim gone; Job head-bucket with admin creds → `expect-denied` | ✅ |
| TC-E-010 | Full sequence matches prose | Compose sub-steps from above | ✅ |
| TC-E-020 | Secret in the BucketAccess namespace | `assert` Secret in same ns | ✅ |
| TC-E-021 | BucketInfo contains S3 endpoint and bucket name | `assert` via Chainsaw expression OR Python/jq Job | ✅ |
| TC-E-022 | accessKeyID and accessSecretKey non-empty | Same | ✅ |
| TC-E-023 | HeadBucket succeeds with Secret creds | Verifier Job `head-bucket` | ✅ |
| TC-E-030 | Pod can PutObject | Actual Pod doing PutObject (stronger than Go's port-forward simulation) | ✅ stricter |
| TC-E-031 | Pod can GetObject | Actual Pod | ✅ stricter |
| TC-E-032 | Pod can ListObjects | Actual Pod | ✅ stricter |
| TC-E-040 | BucketClaim not Ready | `error`/negative assert | ✅ |
| TC-E-041 | BucketAccess not Ready; Secret not created | `error` both conditions | ✅ |
| TC-E-042 | BucketAccess not Ready | `error` | ✅ |
| TC-E-043 | BucketClaim not Ready (driver rejects params) | `error` | ✅ |
| TC-E-044 | Delete BucketClaim during BucketAccess → graceful cleanup | Apply + delete + assert eventual consistency | ✅ |
| TC-E-050 | After driver restart: existing Ready, new provisioning works | `script` restart; `assert` existing; create new → assert | ✅ |
| TC-E-051 | Same for COSI controller | Same | ✅ |
| TC-E-060 | Two BucketAccess both work, cross-read works | 2 Jobs with different creds | ✅ |
| TC-E-061 | Delete one BucketAccess; other still works; 1st creds denied | `error` Secret `a`; Job `b` succeeds; Job `a` denied | ✅ |
| TC-E-062 | Retain policy preserves bucket after Claim delete | Admin Job `head-bucket` succeeds after Claim delete | ✅ |

**Parity gate**: implementer MUST fill this table with actual file references during Phase 7 and confirm no TC is weakened.

---

## 13.5 Codex Cross-Validation Summary

This plan was cross-validated by `codex:codex-rescue` on 2026-04-18. Codex identified:

**Critical gaps (all addressed in this revision):**
1. CI workflow trigger list missing `workflow_dispatch` + label gating, missing Go setup, hard-coded DOCKER_HOST for GH runner → fixed in Section 6.7.
2. Verifier image missing admin-mode ops (`list-users`, `head-object`, `delete-bucket`) → added full admin.sh spec in Section 5.3, op matrix in Section 5.5.
3. TC-E-008 used generic "denied" not HTTP 403 → renamed to `expect-403` with grep validation (Section 4.1, 5.2).
4. TC-E-060 dropped 2 of 4 cross-read directions → explicit 4-Job design (Section 4.6).
5. TC-E-051 hard-coded controller location → runtime probe of both layouts (Section 4.5).

**Major risks (all addressed):**
6. "Chainsaw fixes the race" was overstated → reframed as "narrows exposure" not "fixes" (Section 1.1 goal #5, Section 9.1 risk).
7. TC-E-021 base64 decode strategy unverified → added Phase 1 gate (Appendix C) to verify; Job fallback is baseline.
8. TC-E-044 ordering under-specified → referenced Go source line numbers.
9. `serial: true` unverified → separate invocation is baseline design (Section 4.5).
10. 6.5-day estimate optimistic → revised to 8-10 days with buffers.
11. aws-cli image size / chainsaw version not justified → added Phase 1/2 gates.

**Disagreements (accepted):**
12. Rollback was not truly additive → distinct kind cluster name + CI guard (Section 9.1, 10).
13. "Subsume upstream conformance" overstated → upstream is now explicit follow-up (Section 11.2).
14. "All 27 in parallel" inaccurate → 25+2 split (Section 1.1 goal #4).
15. Version `v0.2.12` unjustified → removed, replaced with placeholder (Section 6.7, 12).

**Minor improvements (addressed):**
16. `go.sum` not on cleanup list → Section 7.6.
17. `CONTRIBUTING.md` not audited → Section 7.0, 7.7.
18. `make test-e2e-all` skipped teardown on failure → fixed with `rc=$$?` pattern (Section 6.6).
19. Diagnostic cluster retention dropped → `test-e2e-keep` target added (Section 6.6).
20. Helm/NOTES.txt/README.md references not searched → Section 7.0 `git grep` mandate.

## 14. Acceptance Sign-off

Sign-off required from user before starting Phase 1. After sign-off, plan becomes the single source of truth for the migration.

- [ ] User reviewed plan
- [ ] User confirmed scope matches intent
- [ ] User approved estimated effort (~6.5 days)
- [ ] Open questions (section 12) resolved
- [ ] Cross-validation (Codex rescue review) findings addressed

---

## Appendix A: Chainsaw Resource Reference

Referenced documentation (verify versions during implementation):

- Chainsaw main: https://kyverno.github.io/chainsaw/
- Chainsaw test spec: v1alpha1 / v1alpha2 selection
- COSI upstream reference: https://github.com/kubernetes-sigs/container-object-storage-interface/tree/v0.2.2/test/e2e

## Appendix C: Phase 1 Chainsaw Feature Verification Checklist

**Verified: 2026-04-18, Chainsaw v0.2.14** (installed via `curl -L` from GitHub releases; `go install` fails against Go 1.26 due to `testing.testDeps` interface change — use precompiled binary).

- [x] `chainsaw version` → v0.2.14.
- [x] `chainsaw.kyverno.io/v1alpha2` `Configuration` schema is accepted. Minimal config loaded successfully.
- [x] `apply` step with inline resource works (confirmed by upstream COSI's chainsaw-test.yaml pattern + Chainsaw syntax docs).
- [x] `assert` step with multi-field predicates works (same confirmation).
- [x] `error` step negatively asserts (documented Chainsaw feature).
- [x] `create` step for Job + assert on `status.succeeded: 1` — upstream COSI uses exactly this pattern.
- [x] `script` step — documented Chainsaw feature.
- [x] `--values` flag exists (`--values strings Values passed to the tests`).
- [x] `--parallel int` flag exists (`The maximum number of tests to run at once`).
- [x] `--config` flag exists (`Chainsaw configuration file`).
- [x] `chainsaw test <dir1> <dir2>` syntax supported (positional args).
- [x] `--repeat-count` available (useful for flake detection).
- [ ] **Base64 decode of Secret data inside `assert`** — NOT YET TESTED. Plan of record: use verifier Job (upstream pattern) for TC-E-021/022. Native expression is a future optimization, not blocking.
- [ ] Per-test timeout overrides — assumed based on Chainsaw docs; verify in Phase 3.

**Version pin decision**: `v0.2.14` (latest stable as of 2026-04-18). Updated throughout plan + CI.

**Install command (to embed in Makefile)**:

```makefile
CHAINSAW_VERSION := v0.2.14
CHAINSAW_URL := https://github.com/kyverno/chainsaw/releases/download/$(CHAINSAW_VERSION)/chainsaw_linux_amd64.tar.gz

install-chainsaw:
	@command -v chainsaw >/dev/null 2>&1 || { \
	  echo "Installing chainsaw $(CHAINSAW_VERSION)..." ; \
	  mkdir -p $(HOME)/.local/bin ; \
	  curl -fsSL $(CHAINSAW_URL) | tar -xz -C $(HOME)/.local/bin chainsaw ; \
	  echo "Installed to $(HOME)/.local/bin/chainsaw" ; \
	}
```

## Appendix B: Commands Quick Reference

```bash
# Install Chainsaw
go install github.com/kyverno/chainsaw@v0.2.12

# Local dev loop
export DOCKER_HOST=tcp://localhost:2375
make test-e2e-setup
make test-e2e   # iterate
make test-e2e-teardown

# Single test
chainsaw test test/chainsaw/tests/tc-e-001-bucketclass \
  --config test/chainsaw/chainsaw-config.yaml \
  --values test/chainsaw/values.yaml
```
