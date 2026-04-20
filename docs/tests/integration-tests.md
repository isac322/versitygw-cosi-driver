# Integration Tests

## Purpose

Integration tests validate the driver against a **real VersityGW instance** without Kubernetes. They prove that:

- S3 bucket operations actually work against VersityGW
- The custom Admin API (user create/delete) works correctly
- Bucket policies with access key ID principals grant real access
- Returned credentials can perform actual S3 operations
- The full grant/revoke lifecycle works end-to-end at the storage layer
- The driver is compatible across VersityGW versions

These tests are the primary defense against **VersityGW version regressions**.

## Scope & Boundaries

**In scope:**
- Real S3 API calls against VersityGW
- Real Admin API calls against VersityGW
- Real gRPC calls to the driver (over Unix socket or bufconn)
- Credential validation (returned keys must actually work for S3)
- Bucket policy correctness (principals, actions, resources verified via GetBucketPolicy)
- Concurrency behavior

**Out of scope:**
- Kubernetes CRDs, controller, sidecar
- COSI resource lifecycle (BucketClaim, BucketAccess, etc.)
- Pod consumption of Secrets

**What is real:** VersityGW process, gRPC server, S3 client, Admin API client

**What is mocked:** Nothing

## Architecture

### VersityGW Test Instance

Each test (or test suite) starts a dedicated VersityGW instance using `testutil.StartVersityGW(t)`. This helper:

1. Starts a VersityGW process with POSIX backend on a temp directory
2. Configures a root admin account (access key + secret key)
3. Enables the Admin API on a random port
4. Enables the S3 API on a random port
5. Configures a multi-user IAM backend (file-based, `--iam-dir`)
6. Returns endpoints and credentials
7. Registers `t.Cleanup` to stop the process and remove temp dirs

```go
type VersityGWInstance struct {
    S3Endpoint    string  // e.g., "http://127.0.0.1:34567"
    AdminEndpoint string  // e.g., "http://127.0.0.1:34568"
    AccessKey     string  // root admin access key
    SecretKey     string  // root admin secret key
    Region        string  // configured region
}

func StartVersityGW(t *testing.T) *VersityGWInstance
```

### Test Setup Pattern

```go
func TestSomething(t *testing.T) {
    t.Parallel()

    vgw := testutil.StartVersityGW(t)

    // Create real S3 client
    s3Client := newS3Client(t, vgw.S3Endpoint, vgw.AccessKey, vgw.SecretKey, vgw.Region)

    // Create real Admin client
    adminClient := newAdminClient(t, vgw.AdminEndpoint, vgw.AccessKey, vgw.SecretKey, vgw.Region)

    // Create provisioner server with real clients
    srv := driver.NewProvisionerServer(s3Client, adminClient, driverConfig)

    // Run tests against srv
}
```

### Directory Structure

```
integration/
    testutil/
        versitygw.go       -- StartVersityGW helper
        s3client.go        -- S3 client factory
        adminclient.go     -- Admin client factory
    bucket_test.go         -- TC-I-001 through TC-I-005
    user_test.go           -- TC-I-010 through TC-I-014
    grant_test.go          -- TC-I-020 through TC-I-027
    revoke_test.go         -- TC-I-030 through TC-I-035
    lifecycle_test.go      -- TC-I-040 through TC-I-041
    credential_test.go     -- TC-I-050 through TC-I-054
    error_test.go          -- TC-I-060 through TC-I-064
    version_compat_test.go -- TC-I-070
```

### Go Conventions

- Every test calls `t.Parallel()`
- Every helper calls `t.Helper()`
- Use `testify/require`
- Integration tests are gated by build tags or by requiring VersityGW binary availability
- Each test gets its own VersityGW instance via `testutil.StartVersityGW(t)` to enable parallel execution
- Use unique bucket names per test (e.g., prefix with test name) to avoid collisions

---

## Test Cases

### Bucket Lifecycle

These tests validate S3 bucket operations via the COSI gRPC interface against real VersityGW.

#### TC-I-001: Create bucket and verify existence

- **Given**: A running VersityGW instance
- **When**: DriverCreateBucket is called with Name = "<test-unique-name>"
- **Then**:
  - gRPC response has no error
  - Response.BucketId is non-empty
  - S3 HeadBucket for the bucket ID returns 200 (bucket exists)

#### TC-I-002: Create same bucket again -- idempotent

- **Given**: A bucket "<test-unique-name>" already created via DriverCreateBucket
- **When**: DriverCreateBucket is called again with the same Name
- **Then**:
  - gRPC response has no error (codes.OK)
  - Response.BucketId matches the first call's BucketId

#### TC-I-003: Delete bucket and verify removal

- **Given**: A bucket created via DriverCreateBucket
- **When**: DriverDeleteBucket is called with the BucketId
- **Then**:
  - gRPC response has no error
  - S3 HeadBucket for the bucket returns 404 (NotFound)

#### TC-I-004: Delete non-existent bucket -- idempotent

- **Given**: No bucket with ID "nonexistent-bucket" exists
- **When**: DriverDeleteBucket is called with BucketId = "nonexistent-bucket"
- **Then**: gRPC response has no error (codes.OK -- idempotent)

#### TC-I-005: Create bucket with parameters

- **Given**: A running VersityGW instance
- **When**: DriverCreateBucket is called with Parameters containing driver-recognized keys
- **Then**:
  - gRPC response has no error
  - Bucket is created with the expected configuration

---

### User Management via Admin API

These tests validate that the driver correctly creates and deletes VersityGW users through the Admin API. These are called indirectly through DriverGrantBucketAccess / DriverRevokeBucketAccess but are also verified independently.

#### TC-I-010: Grant creates user visible in ListUsers

- **Given**: A bucket created via DriverCreateBucket
- **When**: DriverGrantBucketAccess is called
- **Then**:
  - gRPC response has no error
  - Admin API ListUsers includes a user with the access key matching Response.AccountId

#### TC-I-011: Created user has correct role

- **Given**: A successful DriverGrantBucketAccess
- **When**: Admin API ListUsers is called
- **Then**: The created user has role = "user" (not "admin" or "userplus")

#### TC-I-012: Revoke deletes user from ListUsers

- **Given**: A successful DriverGrantBucketAccess with AccountId
- **When**: DriverRevokeBucketAccess is called with that AccountId
- **Then**:
  - gRPC response has no error
  - Admin API ListUsers does NOT include the user with that access key

#### TC-I-013: Revoke non-existent user -- idempotent

- **Given**: No user with access key "nonexistent-akid"
- **When**: DriverRevokeBucketAccess is called with AccountId = "nonexistent-akid" and a valid BucketId
- **Then**: gRPC response has no error (codes.OK -- idempotent)

#### TC-I-014: Created user can authenticate to S3

- **Given**: A successful DriverGrantBucketAccess returning credentials
- **When**: A new S3 client is created using the returned accessKeyID and accessSecretKey
- **Then**: The client can make authenticated S3 requests (e.g., ListBuckets returns without 403)

---

### Access Grant Flow

These tests validate the complete access granting mechanism: user creation, bucket policy, and credential usability.

#### TC-I-020: Granted credentials allow PutObject

- **Given**: A bucket created, access granted via DriverGrantBucketAccess
- **When**: An S3 client using the returned credentials calls PutObject
- **Then**: PutObject succeeds (HTTP 200)

#### TC-I-021: Granted credentials allow GetObject

- **Given**: A bucket with an object written by admin, access granted
- **When**: An S3 client using the returned credentials calls GetObject
- **Then**: GetObject succeeds, returned body matches what was written

#### TC-I-022: Granted credentials allow ListBucket

- **Given**: A bucket with objects, access granted
- **When**: An S3 client using the returned credentials calls ListObjectsV2
- **Then**: ListObjectsV2 succeeds, returns the expected object keys

#### TC-I-023: Granted credentials allow DeleteObject

- **Given**: A bucket with an object written by the granted user, access granted with read-write
- **When**: An S3 client using the returned credentials calls DeleteObject
- **Then**: DeleteObject succeeds

#### TC-I-024: Bucket policy has correct principal after grant

- **Given**: A successful DriverGrantBucketAccess
- **When**: S3 GetBucketPolicy is called (using admin credentials)
- **Then**:
  - Policy exists
  - Policy JSON has a Statement with the granted user's access key ID in Principal.AWS
  - The principal is the raw access key ID string, NOT an ARN

#### TC-I-025: Bucket policy has correct actions after grant

- **Given**: A successful DriverGrantBucketAccess
- **When**: S3 GetBucketPolicy is called
- **Then**:
  - Policy Statement.Action contains the expected S3 actions (e.g., s3:GetObject, s3:PutObject, s3:DeleteObject, s3:ListBucket for read-write)
  - Policy Statement.Resource includes both `arn:aws:s3:::<bucket>` and `arn:aws:s3:::<bucket>/*`

#### TC-I-026: Multiple grants produce independent working credentials

- **Given**: A bucket created, two separate DriverGrantBucketAccess calls (user1, user2)
- **When**: Both users' credentials are used to PutObject
- **Then**:
  - Both PutObject calls succeed
  - Each user can read the other's objects (both have read-write to the same bucket)

#### TC-I-027: Multiple grants accumulate principals in policy

- **Given**: A bucket created, two separate DriverGrantBucketAccess calls
- **When**: S3 GetBucketPolicy is called
- **Then**:
  - Policy has a single Statement (not duplicated)
  - Principal.AWS contains both users' access key IDs

---

### Access Revoke Flow

#### TC-I-030: Revoked credentials fail for S3 PutObject

- **Given**: Access granted then revoked via DriverRevokeBucketAccess
- **When**: The previously-granted S3 client calls PutObject
- **Then**: PutObject fails with 403 AccessDenied

#### TC-I-031: Revoked credentials fail for S3 GetObject

- **Given**: Access granted, an object written, then access revoked
- **When**: The previously-granted S3 client calls GetObject
- **Then**: GetObject fails with 403 AccessDenied

#### TC-I-032: Revoke removes principal from bucket policy

- **Given**: A successful grant then revoke
- **When**: S3 GetBucketPolicy is called
- **Then**:
  - If this was the only principal: policy does not exist (NoSuchBucketPolicy)
  - If other principals remain: the revoked user's access key ID is not in Principal.AWS

#### TC-I-033: Revoke one of multiple -- others still work

- **Given**: Two users granted access (user1, user2), then user1 revoked
- **When**: User2's credentials are used to PutObject and GetObject
- **Then**: Both operations succeed

#### TC-I-034: Revoke one of multiple -- policy updated correctly

- **Given**: Two users granted access, then user1 revoked
- **When**: S3 GetBucketPolicy is called
- **Then**:
  - Policy still exists
  - Principal.AWS contains user2's access key ID
  - Principal.AWS does NOT contain user1's access key ID

#### TC-I-035: Revoke all -- policy fully removed

- **Given**: Two users granted access, then both revoked
- **When**: S3 GetBucketPolicy is called
- **Then**: NoSuchBucketPolicy error (policy deleted entirely)

---

### Full Lifecycle

#### TC-I-040: Complete lifecycle: create -> grant -> use -> revoke -> delete

- **Given**: A running VersityGW instance
- **When**: The following sequence is executed:
  1. DriverCreateBucket with Name = "lifecycle-test"
  2. DriverGrantBucketAccess with the returned BucketId
  3. Use returned credentials to PutObject("key1", "value1")
  4. Use returned credentials to GetObject("key1") -> verify body == "value1"
  5. DriverRevokeBucketAccess with the returned AccountId
  6. Verify credentials no longer work (PutObject returns 403)
  7. DriverDeleteBucket with the BucketId
  8. Verify bucket no longer exists (HeadBucket returns 404)
- **Then**: All steps succeed as described

#### TC-I-041: Multiple buckets, multiple users

- **Given**: A running VersityGW instance
- **When**:
  1. Create bucket-a and bucket-b
  2. Grant user1 access to bucket-a
  3. Grant user2 access to bucket-b
  4. Grant user3 access to both bucket-a and bucket-b
  5. Verify user1 can write to bucket-a but not bucket-b
  6. Verify user2 can write to bucket-b but not bucket-a
  7. Verify user3 can write to both
  8. Revoke user3 from bucket-a
  9. Verify user3 can still write to bucket-b
  10. Cleanup: revoke all, delete all
- **Then**: All steps succeed as described

---

### Credential Format Validation

#### TC-I-050: Returned accessKeyID is a valid VersityGW access key

- **Given**: A successful DriverGrantBucketAccess
- **When**: Inspect Response.Credentials["s3"].Secrets["accessKeyID"]
- **Then**: Value is non-empty and works as an S3 access key for authentication

#### TC-I-051: Returned accessSecretKey is valid

- **Given**: A successful DriverGrantBucketAccess
- **When**: Inspect Response.Credentials["s3"].Secrets["accessSecretKey"]
- **Then**: Value is non-empty and works with the accessKeyID for SigV4 signing

#### TC-I-052: Returned endpoint is the VersityGW S3 endpoint

- **Given**: A VersityGW instance at S3Endpoint = "http://127.0.0.1:<port>"
- **When**: DriverGrantBucketAccess succeeds
- **Then**: Response.Credentials["s3"].Secrets["endpoint"] matches the configured S3 endpoint

#### TC-I-053: Returned region matches configuration

- **Given**: Driver configured with region = "us-east-1"
- **When**: DriverGrantBucketAccess succeeds
- **Then**: Response.Credentials["s3"].Secrets["region"] == "us-east-1"

#### TC-I-054: Protocol response has S3 with correct fields

- **Given**: A successful DriverCreateBucket
- **When**: Inspect Response.BucketInfo
- **Then**:
  - BucketInfo.S3 is non-nil
  - BucketInfo.S3.Region matches configured region
  - BucketInfo.S3.SignatureVersion == S3V4 (VersityGW supports SigV4)

---

### Error Scenarios

#### TC-I-060: Grant access to non-existent bucket

- **Given**: No bucket named "ghost-bucket" exists
- **When**: DriverGrantBucketAccess is called with BucketId = "ghost-bucket"
- **Then**: Error is returned (exact code depends on implementation -- may fail at HeadBucket or PutBucketPolicy stage)

#### TC-I-061: Grant with IAM auth type

- **Given**: A valid bucket
- **When**: DriverGrantBucketAccess is called with AuthenticationType = IAM
- **Then**: Error with gRPC code InvalidArgument, message indicates IAM is unsupported

#### TC-I-062: Revoke with non-existent account and non-existent bucket

- **Given**: Neither bucket nor user exist
- **When**: DriverRevokeBucketAccess is called
- **Then**: Error is nil (idempotent) or appropriate error depending on implementation

#### TC-I-063: Admin API unreachable

- **Given**: Admin endpoint points to a non-listening port
- **When**: DriverGrantBucketAccess is called
- **Then**: Error with appropriate gRPC code (Unavailable or Internal)

#### TC-I-064: S3 API unreachable

- **Given**: S3 endpoint points to a non-listening port
- **When**: DriverCreateBucket is called
- **Then**: Error with appropriate gRPC code (Unavailable or Internal)

#### TC-I-065: Unix socket transport (COSI_ENDPOINT)

- **Given**: A VersityGW instance and a driver configured to listen on a Unix domain socket
- **When**: The driver is started with COSI_ENDPOINT = "unix:///tmp/test-cosi.sock"
- **Then**:
  - The driver creates and listens on `/tmp/test-cosi.sock`
  - A gRPC client connecting to that socket can call DriverGetInfo successfully
  - After driver shutdown, the socket file is cleaned up
- **Note**: COSI spec mandates Unix socket transport. The endpoint MUST end with `.sock`. This test validates the transport layer works correctly with real gRPC, unlike component tests that use bufconn.

#### TC-I-066: VersityGW single-user mode detection

- **Given**: A VersityGW instance started WITHOUT `--iam-dir` (single-user mode, only root account exists)
- **When**: DriverGrantBucketAccess is called
- **Then**:
  - Error is returned with appropriate gRPC code (Unavailable, FailedPrecondition, or Internal)
  - Error message indicates that user management is not available or the Admin API does not support this operation
- **Note**: VersityGW in single-user mode returns `ErrAdminMethodNotSupported` for all admin operations. The driver should detect this and return a meaningful error rather than a cryptic failure.

#### TC-I-067: Delete bucket with active access grants

- **Given**: A bucket created via DriverCreateBucket, access granted via DriverGrantBucketAccess (user exists, policy set)
- **When**: DriverDeleteBucket is called WITHOUT first revoking access
- **Then**: Either:
  - The bucket is deleted (S3 DeleteBucket succeeds -- VersityGW does not prevent this if the bucket is empty)
  - Error is returned if the driver explicitly checks for active access before deleting
- **Verify**:
  - The previously-granted user still exists in VersityGW (if bucket was deleted without cleanup)
  - OR the driver cleaned up the user and policy before deleting the bucket

---

### VersityGW Version Compatibility

#### TC-I-070: Version matrix

- **Given**: CI is configured to run integration tests against multiple VersityGW versions (e.g., latest, latest-1, latest-2)
- **When**: The entire integration test suite runs against each version
- **Then**: All tests pass for every supported VersityGW version

**Implementation note:** This is not a single test function but a CI configuration concern. The CI pipeline should parameterize the VersityGW binary version and run the full integration suite for each. The `testutil.StartVersityGW` helper should accept a version parameter or use the VersityGW binary found in PATH.

Example CI matrix:
```yaml
strategy:
  matrix:
    versitygw-version: ["v1.0.0", "v1.1.0", "v1.2.0", "latest"]
```

---

## Summary

| ID Range         | Area                           | Count |
|------------------|--------------------------------|-------|
| TC-I-001 ~ 005   | Bucket lifecycle              | 5     |
| TC-I-010 ~ 014   | User management (Admin API)   | 5     |
| TC-I-020 ~ 027   | Access grant flow             | 8     |
| TC-I-030 ~ 035   | Access revoke flow            | 6     |
| TC-I-040 ~ 041   | Full lifecycle                | 2     |
| TC-I-050 ~ 054   | Credential format validation  | 5     |
| TC-I-060 ~ 067   | Error scenarios               | 8     |
| TC-I-070          | Version compatibility         | 1     |
| **Total**        |                                | **40**|
