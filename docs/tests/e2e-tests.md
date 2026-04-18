# E2E Tests

## Purpose

E2E tests validate the **full Kubernetes COSI stack**: COSI controller, provisioner sidecar, our driver, and VersityGW, all deployed in a real cluster. They prove that:

- COSI Custom Resources (BucketClass, BucketClaim, BucketAccessClass, BucketAccess) create and reconcile correctly
- The sidecar correctly calls our driver's gRPC RPCs
- Kubernetes Secrets are created with valid credentials
- Pods can consume the Secrets and perform S3 operations
- Cleanup (delete BucketAccess/BucketClaim) properly removes backend resources

These tests are the primary defense against **Kubernetes version and COSI controller/sidecar regressions**.

## Scope & Boundaries

**In scope:**
- COSI CRD creation, reconciliation, and status
- Secret creation and deletion by the COSI sidecar
- Credential usability from within a Pod
- Resource cleanup on deletion
- Error propagation to CRD status fields
- Driver recovery after restart

**Out of scope:**
- Driver internal logic (covered by unit/component)
- S3/Admin API details (covered by integration)
- Performance, scalability, chaos testing

**What is real:** Everything -- Kubernetes cluster, COSI controller, provisioner sidecar, driver, VersityGW

## Architecture

### Cluster Setup

The E2E test environment requires:

1. **Kubernetes cluster** (kind, k3s, or real cluster)
2. **COSI controller** deployed (from `kubernetes-sigs/container-object-storage-interface`)
3. **Provisioner sidecar** deployed as a sidecar to the driver Pod
4. **Driver** deployed as a container (the image built from this project)
5. **VersityGW** deployed as a StatefulSet or external service, with:
   - Multi-user IAM backend enabled
   - Admin API enabled
   - S3 API enabled

### COSI Resources

The following Kubernetes resources are used in tests:

```yaml
# BucketClass -- defines how buckets are provisioned
apiVersion: objectstorage.k8s.io/v1alpha1
kind: BucketClass
metadata:
  name: versitygw-bucket-class
driverName: versitygw.cosi.dev
deletionPolicy: Delete
parameters:
  # driver-specific parameters (if any)

---
# BucketClaim -- requests a bucket
apiVersion: objectstorage.k8s.io/v1alpha1
kind: BucketClaim
metadata:
  name: my-bucket-claim
  namespace: test-ns
spec:
  bucketClassName: versitygw-bucket-class
  protocols:
    - name: s3

---
# BucketAccessClass -- defines how access is granted
apiVersion: objectstorage.k8s.io/v1alpha1
kind: BucketAccessClass
metadata:
  name: versitygw-access-class
driverName: versitygw.cosi.dev
authenticationType: Key

---
# BucketAccess -- requests access to a bucket
apiVersion: objectstorage.k8s.io/v1alpha1
kind: BucketAccess
metadata:
  name: my-bucket-access
  namespace: test-ns
spec:
  bucketClaimName: my-bucket-claim
  bucketAccessClassName: versitygw-access-class
  credentialsSecretName: my-bucket-creds
  # The sidecar creates a Secret named "my-bucket-creds" in namespace "test-ns"
```

### Test Framework

Use Kyverno Chainsaw or a Go-based E2E framework (e.g., `sigs.k8s.io/e2e-framework`) for declarative or programmatic E2E tests. Either approach should:

1. Create test namespaces per test case
2. Apply COSI resources
3. Wait for conditions (with timeout)
4. Verify state in both Kubernetes and VersityGW
5. Clean up resources after each test

### Directory Structure

```
test/e2e/
    setup_test.go          -- test suite setup (cluster access, VersityGW health check)
    lifecycle_test.go      -- TC-E-001 through TC-E-010
    secret_test.go         -- TC-E-020 through TC-E-023
    pod_test.go            -- TC-E-030 through TC-E-032
    error_test.go          -- TC-E-040 through TC-E-043
    recovery_test.go       -- TC-E-050 through TC-E-051
    testdata/
        bucketclass.yaml
        bucketclaim.yaml
        bucketaccessclass.yaml
        bucketaccess.yaml
        consumer-pod.yaml
```

### Go Conventions

- Build tag: `//go:build e2e`
- Use `t.Parallel()` where tests are independent (different namespaces)
- Use `t.Helper()` in helpers
- Use `testify/require` for assertions
- Use `wait.PollUntilContextTimeout` or similar for condition polling with timeout
- Default timeout for resource reconciliation: 60 seconds
- Each test creates its own namespace and cleans up after

---

## Test Cases

### COSI Resource Lifecycle

#### TC-E-001: BucketClass creation succeeds

- **Given**: A Kubernetes cluster with COSI controller installed
- **When**: A BucketClass resource with driverName = "versitygw.cosi.dev" is created
- **Then**: The BucketClass resource exists without errors in its status

#### TC-E-002: BucketClaim creates a Bucket in VersityGW

- **Given**: A BucketClass exists
- **When**: A BucketClaim referencing the BucketClass is created in a test namespace
- **Then**:
  - Within 60s, BucketClaim status becomes Ready
  - A Bucket CRD is created by the COSI controller
  - The corresponding S3 bucket exists in VersityGW (verified via HeadBucket using admin credentials)

#### TC-E-003: BucketClaim status reflects bucket ID

- **Given**: A BucketClaim that has been provisioned
- **When**: BucketClaim status is inspected
- **Then**: Status contains a reference to the Bucket CRD, and Bucket.status.bucketID is non-empty

#### TC-E-004: BucketAccessClass creation succeeds

- **Given**: A Kubernetes cluster with COSI controller installed
- **When**: A BucketAccessClass with driverName = "versitygw.cosi.dev" and authenticationType = Key
- **Then**: The BucketAccessClass resource exists without errors

#### TC-E-005: BucketAccess creates Secret with credentials

- **Given**: A BucketClaim in Ready state
- **When**: A BucketAccess referencing the BucketClaim, BucketAccessClass, and credentialsSecretName = "test-creds" is created
- **Then**:
  - Within 60s, BucketAccess status becomes Ready
  - A Secret named "test-creds" exists in the same namespace
  - The Secret contains the BucketInfo data

#### TC-E-006: BucketAccess status reflects account ID

- **Given**: A BucketAccess that has been provisioned
- **When**: BucketAccess status is inspected
- **Then**: Status reflects the provisioned state, and the underlying Bucket access has been granted

#### TC-E-007: Delete BucketAccess removes Secret

- **Given**: A BucketAccess in Ready state with an associated Secret
- **When**: The BucketAccess resource is deleted
- **Then**:
  - Within 60s, the Secret "test-creds" no longer exists
  - The VersityGW user is removed (Admin API ListUsers does not include the account)
  - The bucket policy no longer includes the user's principal

#### TC-E-008: Delete BucketAccess revokes VersityGW access

- **Given**: A BucketAccess that has been deleted
- **When**: The previously-returned credentials are used to call S3 PutObject
- **Then**: PutObject fails with 403 (access revoked at the VersityGW level)

#### TC-E-009: Delete BucketClaim removes bucket from VersityGW

- **Given**: All BucketAccess resources for a BucketClaim have been deleted, BucketClaim exists
- **When**: The BucketClaim is deleted (with deletionPolicy = Delete on the BucketClass)
- **Then**:
  - Within 60s, the BucketClaim is gone
  - The S3 bucket no longer exists in VersityGW (HeadBucket returns 404)

#### TC-E-010: Full lifecycle in order

- **Given**: A clean test namespace
- **When**: The following sequence is executed:
  1. Create BucketClass (if not cluster-scoped and shared)
  2. Create BucketAccessClass (if not cluster-scoped and shared)
  3. Create BucketClaim -> wait for Ready
  4. Create BucketAccess -> wait for Ready, Secret appears
  5. Verify Secret credentials work (PutObject, GetObject via an S3 client created outside the cluster)
  6. Delete BucketAccess -> wait for Secret deletion
  7. Verify credentials no longer work
  8. Delete BucketClaim -> wait for bucket deletion
  9. Verify bucket gone from VersityGW
- **Then**: All steps succeed as described

---

### Secret Validation

#### TC-E-020: Secret is in the correct namespace

- **Given**: BucketAccess with credentialsSecretName = "creds" in namespace "test-ns"
- **When**: BucketAccess becomes Ready
- **Then**: Secret "creds" exists in namespace "test-ns" (not in another namespace)

#### TC-E-021: Secret contains BucketInfo with S3 endpoint

- **Given**: A provisioned BucketAccess
- **When**: The Secret data is read
- **Then**:
  - The Secret contains a key for BucketInfo (the exact key depends on the COSI sidecar version, typically `BucketInfo`)
  - The BucketInfo JSON contains the S3 endpoint, region, and bucket name

#### TC-E-022: Secret contains valid S3 credentials

- **Given**: A provisioned BucketAccess
- **When**: The Secret data is read and parsed
- **Then**:
  - The data contains accessKeyID and accessSecretKey
  - Both are non-empty strings

#### TC-E-023: Credentials from Secret connect to VersityGW

- **Given**: A provisioned BucketAccess, Secret read
- **When**: An S3 client is created using the credentials from the Secret (endpoint, accessKeyID, accessSecretKey, region)
- **Then**: The client can successfully call HeadBucket for the provisioned bucket

---

### Pod Consumption

These tests verify that a Pod can mount the Secret and use the credentials to perform S3 operations.

#### TC-E-030: Pod can PutObject using Secret credentials

- **Given**: A BucketAccess in Ready state, Secret "creds" exists
- **When**: A Pod is created that:
  1. Mounts Secret "creds" as environment variables or volume
  2. Runs an S3 client (e.g., aws cli or a simple Go binary) to PutObject("test-key", "test-value")
- **Then**:
  - Pod completes successfully (exit code 0)
  - The object "test-key" exists in the VersityGW bucket

#### TC-E-031: Pod can GetObject using Secret credentials

- **Given**: A bucket with object "test-key" written by admin, BucketAccess Ready
- **When**: A Pod runs an S3 client to GetObject("test-key")
- **Then**:
  - Pod completes successfully
  - Retrieved value matches what was written

#### TC-E-032: Pod can ListObjects using Secret credentials

- **Given**: A bucket with multiple objects, BucketAccess Ready
- **When**: A Pod runs an S3 client to ListObjectsV2
- **Then**:
  - Pod completes successfully
  - Listed keys match expected objects

---

### Error Scenarios

#### TC-E-040: BucketClaim with non-existent BucketClass

- **Given**: No BucketClass named "nonexistent-class" exists
- **When**: A BucketClaim referencing "nonexistent-class" is created
- **Then**: BucketClaim does not become Ready (status reflects the error)

#### TC-E-041: BucketAccess with IAM auth type

- **Given**: A BucketAccessClass with authenticationType = IAM
- **When**: A BucketAccess referencing this class is created
- **Then**:
  - BucketAccess does not become Ready
  - Status message indicates IAM authentication is not supported
  - No Secret is created

#### TC-E-042: BucketAccess for non-existent BucketClaim

- **Given**: No BucketClaim named "ghost-claim" exists
- **When**: A BucketAccess referencing "ghost-claim" is created
- **Then**: BucketAccess does not become Ready (status reflects the error)

#### TC-E-043: Invalid driver parameters in BucketClass

- **Given**: A BucketClass with invalid parameters (e.g., unrecognized key that the driver rejects)
- **When**: A BucketClaim referencing this BucketClass is created
- **Then**: BucketClaim does not become Ready (status reflects the error from the driver)

#### TC-E-044: Delete BucketClaim while BucketAccess still exists

- **Given**: BucketClaim and BucketAccess both in Ready state
- **When**: BucketClaim is deleted WITHOUT first deleting BucketAccess
- **Then**: Either:
  - BucketClaim deletion is blocked/deferred until BucketAccess is removed (via finalizer), OR
  - BucketClaim deletion proceeds, and BucketAccess enters an error state
- **Verify**: The system does not leave orphaned resources (no VersityGW user without a bucket, no dangling Secret)
- **Note**: This tests the ordering guarantees of the COSI controller. The expected behavior depends on the COSI controller version, but the driver should not panic or leave inconsistent state regardless.

---

### Multi-Access Scenarios

These tests validate real deployment patterns where multiple consumers access the same bucket. Integration tests verify this at the gRPC level, but E2E tests must verify the full Kubernetes flow: multiple BucketAccess CRDs, multiple Secrets, and independent lifecycle management.

#### TC-E-060: Multiple BucketAccess to same BucketClaim

- **Given**: A BucketClaim in Ready state
- **When**:
  1. Create BucketAccess "access-1" with credentialsSecretName = "creds-1"
  2. Create BucketAccess "access-2" with credentialsSecretName = "creds-2"
  3. Wait for both to become Ready
- **Then**:
  - Secret "creds-1" and Secret "creds-2" both exist
  - Both Secrets contain different access key IDs (each user is independent)
  - Credentials from "creds-1" can PutObject and GetObject
  - Credentials from "creds-2" can PutObject and GetObject
  - Objects written by access-1 are readable by access-2 (same bucket)

#### TC-E-061: Delete one BucketAccess while others remain active

- **Given**: Two BucketAccess resources ("access-1", "access-2") for the same BucketClaim, both Ready
- **When**: BucketAccess "access-1" is deleted
- **Then**:
  - Secret "creds-1" is removed
  - Secret "creds-2" still exists
  - Credentials from "creds-1" can no longer access the bucket (403)
  - Credentials from "creds-2" still work (PutObject, GetObject succeed)
  - BucketAccess "access-2" remains in Ready state (unaffected)
  - VersityGW bucket policy still has access-2's principal but NOT access-1's

#### TC-E-062: BucketClass with deletionPolicy=Retain

- **Given**: A BucketClass with deletionPolicy = Retain
- **When**:
  1. Create BucketClaim → wait for Ready
  2. Note the bucket ID from VersityGW
  3. Delete all BucketAccess resources
  4. Delete the BucketClaim
- **Then**:
  - BucketClaim is deleted from Kubernetes
  - The S3 bucket STILL EXISTS in VersityGW (HeadBucket returns 200)
  - Manual cleanup is required (admin must delete the bucket from VersityGW)
- **Note**: This is the opposite of TC-E-009 which uses deletionPolicy=Delete. Both policies must be tested to verify the driver respects the policy.

---

### Recovery Scenarios

#### TC-E-050: Driver restart -- existing resources still functional

- **Given**: BucketClaim and BucketAccess both in Ready state, credentials working
- **When**: The driver Pod is restarted (kubectl delete pod)
- **Then**:
  - After driver Pod restarts, existing credentials still work for S3 operations
  - No BucketClaim or BucketAccess status changes to non-Ready
  - New BucketClaim/BucketAccess can still be provisioned

#### TC-E-051: COSI controller restart -- resources reconcile

- **Given**: BucketClaim and BucketAccess in Ready state
- **When**: The COSI controller Pod is restarted
- **Then**:
  - After restart, existing resources remain in Ready state
  - New provisioning requests work normally

---

## Version Compatibility Notes

E2E tests should be run against a version matrix of:

| Component           | Versions to Test                       |
|---------------------|----------------------------------------|
| Kubernetes          | latest, latest-1, latest-2 minor       |
| COSI controller     | latest release                         |
| VersityGW           | latest, supported previous versions    |

CI pipeline example:
```yaml
strategy:
  matrix:
    k8s-version: ["1.30", "1.31", "1.32"]
    versitygw-version: ["latest", "v1.1.0"]
```

---

## Summary

| ID Range         | Area                     | Count |
|------------------|--------------------------|-------|
| TC-E-001 ~ 010   | COSI resource lifecycle | 10    |
| TC-E-020 ~ 023   | Secret validation       | 4     |
| TC-E-030 ~ 032   | Pod consumption         | 3     |
| TC-E-040 ~ 044   | Error scenarios         | 5     |
| TC-E-050 ~ 051   | Recovery scenarios      | 2     |
| TC-E-060 ~ 062   | Multi-access scenarios  | 3     |
| **Total**        |                          | **27**|
