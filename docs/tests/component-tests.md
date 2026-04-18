# Component Tests

## Purpose

Component tests validate the **gRPC server behavior** with mocked backend dependencies. They test the provisioner's RPC contract: correct responses, idempotency semantics, error code compliance, request validation, cleanup on partial failure, and unsupported feature rejection.

These tests catch:
- COSI spec contract violations (wrong error codes, broken idempotency)
- Missing request validation
- Incorrect response field population
- Failure to clean up on partial errors (e.g., user created but policy failed)
- Incorrect handling of unsupported features (e.g., IAM auth)

## Scope & Boundaries

**In scope:**
- All 5 COSI gRPC RPCs (DriverGetInfo, DriverCreateBucket, DriverDeleteBucket, DriverGrantBucketAccess, DriverRevokeBucketAccess)
- Request validation at the RPC handler level
- Response field correctness
- gRPC status code compliance
- Idempotency behavior
- Partial failure cleanup
- Context cancellation handling

**Out of scope:**
- Actual S3 API calls (mocked)
- Actual Admin API calls (mocked)
- Kubernetes resources
- Credential usability (tested in integration)
- Bucket policy correctness at the S3 level (tested in integration)

**What is real:** gRPC server (in-process, over bufconn or similar)

**What is mocked:** S3 client interface, Admin API client interface

## Architecture

### Mock Interfaces

The provisioner depends on two interfaces that are mocked in component tests:

```go
// S3API defines the S3 operations used by the provisioner.
type S3API interface {
    CreateBucket(ctx context.Context, params *s3.CreateBucketInput, ...) (*s3.CreateBucketOutput, error)
    DeleteBucket(ctx context.Context, params *s3.DeleteBucketInput, ...) (*s3.DeleteBucketOutput, error)
    HeadBucket(ctx context.Context, params *s3.HeadBucketInput, ...) (*s3.HeadBucketOutput, error)
    PutBucketPolicy(ctx context.Context, params *s3.PutBucketPolicyInput, ...) (*s3.PutBucketPolicyOutput, error)
    GetBucketPolicy(ctx context.Context, params *s3.GetBucketPolicyInput, ...) (*s3.GetBucketPolicyOutput, error)
    DeleteBucketPolicy(ctx context.Context, params *s3.DeleteBucketPolicyInput, ...) (*s3.DeleteBucketPolicyOutput, error)
}

// AdminAPI defines the VersityGW Admin API operations used by the provisioner.
type AdminAPI interface {
    CreateUser(ctx context.Context, accessKey, secretKey, role string) error
    DeleteUser(ctx context.Context, accessKey string) error
    ListUsers(ctx context.Context) ([]Account, error)
}
```

### Mock Behavior Specification

Mock implementations should:
1. Record all calls (method name, arguments) for verification
2. Return preconfigured responses per scenario
3. Support error injection per call
4. Support call counting for verifying cleanup behavior

Recommended approach: Use function fields in a struct (no external mock library required).

```go
type MockS3Client struct {
    CreateBucketFunc      func(ctx context.Context, params *s3.CreateBucketInput, ...) (*s3.CreateBucketOutput, error)
    DeleteBucketFunc      func(ctx context.Context, params *s3.DeleteBucketInput, ...) (*s3.DeleteBucketOutput, error)
    HeadBucketFunc        func(ctx context.Context, params *s3.HeadBucketInput, ...) (*s3.HeadBucketOutput, error)
    PutBucketPolicyFunc   func(ctx context.Context, params *s3.PutBucketPolicyInput, ...) (*s3.PutBucketPolicyOutput, error)
    GetBucketPolicyFunc   func(ctx context.Context, params *s3.GetBucketPolicyInput, ...) (*s3.GetBucketPolicyOutput, error)
    DeleteBucketPolicyFunc func(ctx context.Context, params *s3.DeleteBucketPolicyInput, ...) (*s3.DeleteBucketPolicyOutput, error)
}

type MockAdminClient struct {
    CreateUserFunc func(ctx context.Context, accessKey, secretKey, role string) error
    DeleteUserFunc func(ctx context.Context, accessKey string) error
    ListUsersFunc  func(ctx context.Context) ([]Account, error)
}
```

### Test Setup Pattern

```go
func setupTest(t *testing.T, s3Mock *MockS3Client, adminMock *MockAdminClient) cosi.ProvisionerClient {
    t.Helper()
    // 1. Create provisioner server with mocked deps
    // 2. Start in-process gRPC server (bufconn)
    // 3. Return gRPC client connected to the in-process server
    // 4. t.Cleanup to stop the server
}
```

### Directory Structure

```
internal/driver/
    identity_test.go      -- TC-C-001 through TC-C-002
    provisioner_test.go   -- TC-C-010 through TC-C-070
    mock_test.go          -- MockS3Client, MockAdminClient
```

### Go Conventions

- Every test function and subtest calls `t.Parallel()`
- Every test helper calls `t.Helper()`
- Use `require` from `github.com/stretchr/testify/require`
- Use `status.Code(err)` from `google.golang.org/grpc/status` to assert gRPC error codes
- Table-driven tests for scenarios sharing the same setup pattern

---

## Test Cases

### DriverGetInfo

#### TC-C-001: Returns configured driver name

- **Given**: Identity server configured with name "versitygw.cosi.dev"
- **When**: DriverGetInfo RPC is called with empty request
- **Then**:
  - Error is nil
  - Response.Name == "versitygw.cosi.dev"

#### TC-C-002: Returns non-empty name

- **Given**: Identity server configured with any valid name
- **When**: DriverGetInfo RPC is called
- **Then**:
  - Response.Name is non-empty
  - Response.Name length <= 63

---

### DriverCreateBucket

#### TC-C-010: Successful bucket creation

- **Given**: S3 mock CreateBucket returns success
- **When**: DriverCreateBucket is called with Name = "test-bucket"
- **Then**:
  - Error is nil
  - Response.BucketId is non-empty
  - Response.BucketInfo is non-nil with S3 protocol set
  - S3 mock CreateBucket was called once with bucket name derived from "test-bucket"

#### TC-C-011: Idempotent creation -- same name and params

- **Given**: S3 mock CreateBucket returns BucketAlreadyOwnedByYou error
- **When**: DriverCreateBucket is called with Name = "existing-bucket"
- **Then**:
  - Error is nil (codes.OK)
  - Response.BucketId matches the existing bucket's ID
  - Response.BucketInfo has correct S3 protocol

#### TC-C-012: Conflict -- same name, different params

- **Given**: S3 mock CreateBucket returns BucketAlreadyExists error (owned by different account)
- **When**: DriverCreateBucket is called
- **Then**:
  - Error has gRPC code AlreadyExists
  - Error message is human-readable

#### TC-C-013: Missing name returns INVALID_ARGUMENT

- **Given**: Any mock setup
- **When**: DriverCreateBucket is called with Name = ""
- **Then**:
  - Error has gRPC code InvalidArgument
  - S3 mock CreateBucket was NOT called

#### TC-C-014: S3 CreateBucket failure propagates

- **Given**: S3 mock CreateBucket returns a generic AWS error (e.g., InternalServerError)
- **When**: DriverCreateBucket is called
- **Then**:
  - Error is non-nil with appropriate gRPC code (Internal)
  - Error message wraps the original S3 error

#### TC-C-015: Parameters passed to provisioner

- **Given**: S3 mock CreateBucket returns success
- **When**: DriverCreateBucket is called with Parameters = {"key": "value"}
- **Then**:
  - The provisioner receives and can use the parameters
  - (Exact behavior depends on which parameters are recognized)

#### TC-C-016: Response Protocol has S3 with correct fields

- **Given**: Driver configured with region "us-east-1" and signature version S3V4
- **When**: DriverCreateBucket succeeds
- **Then**:
  - Response.BucketInfo.S3 is non-nil
  - Response.BucketInfo.S3.Region == "us-east-1"
  - Response.BucketInfo.S3.SignatureVersion == S3V4
  - Response.BucketInfo.AzureBlob is nil
  - Response.BucketInfo.Gcs is nil

#### TC-C-017: S3 client called with correct bucket name

- **Given**: S3 mock records call arguments
- **When**: DriverCreateBucket is called with Name = "test-bucket"
- **Then**: S3 mock CreateBucket was called with `Bucket` field matching the expected bucket ID derived from "test-bucket"

---

### DriverDeleteBucket

#### TC-C-020: Successful bucket deletion

- **Given**: S3 mock DeleteBucket returns success
- **When**: DriverDeleteBucket is called with BucketId = "test-bucket"
- **Then**:
  - Error is nil
  - S3 mock DeleteBucket was called once with correct bucket name

#### TC-C-021: Idempotent -- bucket already deleted

- **Given**: S3 mock DeleteBucket returns NoSuchBucket error
- **When**: DriverDeleteBucket is called
- **Then**:
  - Error is nil (codes.OK -- idempotent)

#### TC-C-022: Missing bucket_id returns INVALID_ARGUMENT

- **Given**: Any mock setup
- **When**: DriverDeleteBucket is called with BucketId = ""
- **Then**:
  - Error has gRPC code InvalidArgument
  - S3 mock DeleteBucket was NOT called

#### TC-C-023: S3 DeleteBucket failure propagates

- **Given**: S3 mock DeleteBucket returns a generic error
- **When**: DriverDeleteBucket is called
- **Then**:
  - Error is non-nil with appropriate gRPC code

#### TC-C-024: Non-empty bucket error

- **Given**: S3 mock DeleteBucket returns BucketNotEmpty error
- **When**: DriverDeleteBucket is called
- **Then**:
  - Error is non-nil with appropriate gRPC code (FailedPrecondition or Internal)
  - Error message indicates bucket is not empty

---

### DriverGrantBucketAccess

This is the most complex RPC. The driver must:
1. Generate access key and secret key for the new user
2. Create user via Admin API
3. Set/update bucket policy with the new user's access key ID as principal
4. Return the account_id and credentials

#### TC-C-030: Successful grant returns account_id and credentials

- **Given**: Admin mock CreateUser returns success, S3 mock GetBucketPolicy returns NoSuchBucketPolicy, S3 mock PutBucketPolicy returns success
- **When**: DriverGrantBucketAccess is called with BucketId = "test-bucket", Name = "user1", AuthenticationType = Key
- **Then**:
  - Error is nil
  - Response.AccountId is non-empty
  - Response.Credentials has key "s3"
  - Response.Credentials["s3"].Secrets contains "accessKeyID", "accessSecretKey", "endpoint", "region"

#### TC-C-031: Admin API called with correct user parameters

- **Given**: Admin mock records call arguments
- **When**: DriverGrantBucketAccess succeeds
- **Then**: Admin mock CreateUser was called with a non-empty accessKey, non-empty secretKey, and role = "user"

#### TC-C-032: Bucket policy set with access key ID as principal

- **Given**: S3 mock PutBucketPolicy records the policy JSON
- **When**: DriverGrantBucketAccess succeeds
- **Then**:
  - PutBucketPolicy was called
  - The policy JSON contains the created user's access key ID in Principal.AWS
  - The policy JSON does NOT use ARN format for the principal

#### TC-C-033: Credentials map has correct structure

- **Given**: Admin mock and S3 mock return success
- **When**: DriverGrantBucketAccess succeeds
- **Then**:
  - Credentials map has exactly 1 entry with key "s3"
  - Secrets map has exactly 4 entries: accessKeyID, accessSecretKey, endpoint, region
  - All values are non-empty strings

#### TC-C-034: Idempotent grant -- same request returns same credentials

- **Given**: Admin mock CreateUser returns "user already exists" (conflict), and the existing user can be looked up
- **When**: DriverGrantBucketAccess is called with the same Name as an existing grant
- **Then**:
  - Error is nil (codes.OK)
  - Response contains valid credentials for the existing user

#### TC-C-035: Conflict -- same name, different params

- **Given**: A previous grant exists with Name = "user1" but different parameters
- **When**: DriverGrantBucketAccess is called with Name = "user1" and different params
- **Then**: Error has gRPC code AlreadyExists

#### TC-C-036: IAM auth type returns INVALID_ARGUMENT

- **Given**: Any mock setup
- **When**: DriverGrantBucketAccess is called with AuthenticationType = IAM
- **Then**:
  - Error has gRPC code InvalidArgument
  - Error message indicates IAM authentication is not supported
  - Admin mock CreateUser was NOT called
  - S3 mock PutBucketPolicy was NOT called

#### TC-C-037: Unknown auth type (0) returns INVALID_ARGUMENT

- **Given**: Any mock setup
- **When**: DriverGrantBucketAccess is called with AuthenticationType = UnknownAuthenticationType (0)
- **Then**:
  - Error has gRPC code InvalidArgument
  - No backend calls made

#### TC-C-038: Missing bucket_id returns INVALID_ARGUMENT

- **Given**: Any mock setup
- **When**: DriverGrantBucketAccess is called with BucketId = ""
- **Then**:
  - Error has gRPC code InvalidArgument
  - No backend calls made

#### TC-C-039: Missing name returns INVALID_ARGUMENT

- **Given**: Any mock setup
- **When**: DriverGrantBucketAccess is called with Name = ""
- **Then**:
  - Error has gRPC code InvalidArgument
  - No backend calls made

#### TC-C-040: Admin CreateUser failure propagates

- **Given**: Admin mock CreateUser returns error
- **When**: DriverGrantBucketAccess is called
- **Then**:
  - Error is non-nil with appropriate gRPC code
  - S3 mock PutBucketPolicy was NOT called (aborted before policy step)

#### TC-C-041: PutBucketPolicy failure triggers user cleanup

- **Given**: Admin mock CreateUser returns success, S3 mock PutBucketPolicy returns error
- **When**: DriverGrantBucketAccess is called
- **Then**:
  - Error is non-nil with appropriate gRPC code
  - Admin mock DeleteUser was called to clean up the created user (rollback)

#### TC-C-042: Multiple grants to same bucket accumulate principals

- **Given**:
  - First grant: S3 mock GetBucketPolicy returns NoSuchBucketPolicy, grant succeeds
  - Second grant: S3 mock GetBucketPolicy returns the policy from the first grant
- **When**: Two different DriverGrantBucketAccess calls for the same bucket
- **Then**:
  - Both succeed
  - The PutBucketPolicy call for the second grant contains both principals in the policy

#### TC-C-043: Parameters passed through to provisioner

- **Given**: S3 mock and Admin mock return success
- **When**: DriverGrantBucketAccess is called with Parameters = {"key": "value"}
- **Then**: The provisioner receives and can use the parameters (exact behavior is driver-specific)

---

### DriverRevokeBucketAccess

The driver must:
1. Remove the principal from the bucket policy
2. Delete the user via Admin API

#### TC-C-050: Successful revoke

- **Given**: S3 mock GetBucketPolicy returns a policy with the account's principal, S3 mock PutBucketPolicy/DeleteBucketPolicy returns success, Admin mock DeleteUser returns success
- **When**: DriverRevokeBucketAccess is called with BucketId = "test-bucket", AccountId = "AKID001"
- **Then**:
  - Error is nil
  - S3 mock GetBucketPolicy was called
  - S3 mock PutBucketPolicy or DeleteBucketPolicy was called (policy updated)
  - Admin mock DeleteUser was called with the correct access key

#### TC-C-051: Idempotent -- already revoked

- **Given**: S3 mock GetBucketPolicy returns a policy WITHOUT the account's principal (or NoSuchBucketPolicy), Admin mock DeleteUser returns "user not found" (or success)
- **When**: DriverRevokeBucketAccess is called
- **Then**: Error is nil (codes.OK)

#### TC-C-052: Missing bucket_id returns INVALID_ARGUMENT

- **Given**: Any mock setup
- **When**: DriverRevokeBucketAccess is called with BucketId = ""
- **Then**:
  - Error has gRPC code InvalidArgument
  - No backend calls made

#### TC-C-053: Missing account_id returns INVALID_ARGUMENT

- **Given**: Any mock setup
- **When**: DriverRevokeBucketAccess is called with AccountId = ""
- **Then**:
  - Error has gRPC code InvalidArgument
  - No backend calls made

#### TC-C-054: Admin DeleteUser failure propagates

- **Given**: S3 mock policy update succeeds, Admin mock DeleteUser returns a non-retryable error
- **When**: DriverRevokeBucketAccess is called
- **Then**: Error is non-nil with appropriate gRPC code

#### TC-C-055: S3 policy update failure propagates

- **Given**: S3 mock GetBucketPolicy succeeds but PutBucketPolicy returns error
- **When**: DriverRevokeBucketAccess is called
- **Then**: Error is non-nil with appropriate gRPC code

#### TC-C-056: Revoke one of multiple principals

- **Given**: S3 mock GetBucketPolicy returns a policy with principals ["AKID001", "AKID002"]
- **When**: DriverRevokeBucketAccess is called with AccountId = "AKID001"
- **Then**:
  - S3 mock PutBucketPolicy was called (NOT DeleteBucketPolicy)
  - The updated policy still contains "AKID002" but NOT "AKID001"
  - Admin mock DeleteUser was called with "AKID001"

#### TC-C-057: Revoke last principal removes entire policy

- **Given**: S3 mock GetBucketPolicy returns a policy with single principal "AKID001"
- **When**: DriverRevokeBucketAccess is called with AccountId = "AKID001"
- **Then**:
  - S3 mock DeleteBucketPolicy was called (entire policy removed)
  - Admin mock DeleteUser was called with "AKID001"

---

### Cross-Cutting Concerns

#### TC-C-060: Context cancellation handling

- **Given**: A context that is cancelled before the RPC completes
- **When**: Any long-running RPC is called (DriverCreateBucket, DriverGrantBucketAccess)
- **Then**:
  - Error has gRPC code Canceled
  - No partial state is left (cleanup runs even on cancellation)

#### TC-C-061: Context deadline exceeded

- **Given**: A context with a very short deadline (e.g., 1ns)
- **When**: Any RPC is called
- **Then**:
  - Error has gRPC code DeadlineExceeded

#### TC-C-062: Nil request handling

- **Given**: Any mock setup
- **When**: Any RPC is called with a nil request
- **Then**: Error has gRPC code InvalidArgument (not a panic)

#### TC-C-063: Concurrent CreateBucket on same name returns ABORTED or serializes

- **Given**: S3 mock CreateBucket introduces a 100ms delay
- **When**: Two goroutines call DriverCreateBucket concurrently with the same Name
- **Then**: Both calls return either:
  - Both succeed (codes.OK) with the same BucketId (serialized execution), OR
  - One succeeds and the other returns codes.Aborted (concurrent conflict detected)
- **Note**: COSI spec requires ABORTED for concurrent operations on the same resource. The driver may choose to serialize instead. Either behavior is acceptable, but the driver MUST NOT return inconsistent BucketIds or leave partial state.

#### TC-C-064: Delete bucket while access grants exist

- **Given**: S3 mock is configured to track state: a bucket exists with an active bucket policy (access has been granted)
- **When**: DriverDeleteBucket is called for that bucket
- **Then**: Either:
  - Error with gRPC code FailedPrecondition (driver refuses to delete a bucket with active access), OR
  - The bucket is deleted successfully (driver does not check for active access -- the caller is responsible for revoking first)
- **Note**: The COSI spec does not prescribe behavior here. This test documents the driver's chosen behavior. The sidecar is expected to revoke access before deleting the bucket, but the driver should handle the case gracefully regardless.

#### TC-C-065: Error responses have non-empty human-readable message

- **Given**: Any scenario that produces a gRPC error (e.g., TC-C-013 empty name)
- **When**: The error is returned
- **Then**:
  - `status.Convert(err).Message()` is non-empty
  - The message is human-readable (not an empty string, not a raw stack trace)
- **Spec Reference**: COSI spec: "The `message` field MUST contain a human-readable description of the error."

#### TC-C-066: Error responses have empty details field

- **Given**: Any scenario that produces a gRPC error
- **When**: The error is returned
- **Then**: `status.Convert(err).Details()` is empty (length 0)
- **Spec Reference**: COSI spec: "The `details` field MUST be empty."

---

## Summary

| ID Range         | Area                              | Count |
|------------------|-----------------------------------|-------|
| TC-C-001 ~ 002   | DriverGetInfo                    | 2     |
| TC-C-010 ~ 017   | DriverCreateBucket               | 8     |
| TC-C-020 ~ 024   | DriverDeleteBucket               | 5     |
| TC-C-030 ~ 043   | DriverGrantBucketAccess          | 14    |
| TC-C-050 ~ 057   | DriverRevokeBucketAccess         | 8     |
| TC-C-060 ~ 066   | Cross-cutting concerns           | 7     |
| **Total**        |                                   | **44**|
