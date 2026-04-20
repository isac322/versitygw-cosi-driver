# Unit Tests

## Purpose

Unit tests validate **pure logic** in isolation with zero external dependencies. They run in milliseconds and catch:

- Configuration parsing and validation errors
- Bucket policy JSON construction mistakes
- Admin API request serialization errors
- COSI credential format mapping bugs
- Error code translation mistakes
- Input validation gaps

## Scope & Boundaries

**In scope:**
- Functions that transform data (parse, validate, serialize, map)
- Functions that construct requests or responses
- Functions that translate error types

**Out of scope:**
- Network I/O (S3 API calls, Admin API calls, gRPC)
- Anything requiring a running VersityGW or gRPC server
- Kubernetes resources

**Mocking:** Minimal to none. Unit tests should test pure functions. If a function requires an interface dependency, that dependency should be a simple interface with a trivial fake, not a complex mock. Prefer extracting pure logic into standalone functions.

## Architecture

### Directory Structure

```
pkg/
  config/
    config.go          -- Config struct, Parse, Validate
    config_test.go     -- TC-U-001 through TC-U-008
  identity/
    identity.go        -- driver name validation
    identity_test.go   -- TC-U-010 through TC-U-015
  bucket/
    id.go              -- bucket ID generation/parsing
    id_test.go         -- TC-U-020 through TC-U-024
    name.go            -- bucket name validation
    name_test.go       -- TC-U-025 through TC-U-030
  policy/
    policy.go          -- bucket policy construction
    policy_test.go     -- TC-U-040 through TC-U-048
  admin/
    request.go         -- Admin API request building, XML serialization
    request_test.go    -- TC-U-050 through TC-U-053
  credential/
    credential.go      -- COSI credential format mapping
    credential_test.go -- TC-U-060 through TC-U-064
  protocol/
    protocol.go        -- COSI Protocol message construction
    protocol_test.go   -- TC-U-070 through TC-U-072
  errors/
    mapping.go         -- S3/Admin errors -> gRPC status codes
    mapping_test.go    -- TC-U-080 through TC-U-089
  validation/
    request.go         -- gRPC request validation
    request_test.go    -- TC-U-090 through TC-U-097
```

> Note: The directory structure above is a logical grouping. The actual package layout may differ. The important thing is that each group of functions is testable in isolation.

### Go Conventions

- Every test function calls `t.Parallel()`
- Every test helper calls `t.Helper()`
- Use `require` from `github.com/stretchr/testify/require` for all assertions
- Use table-driven tests for parameterized scenarios
- No `//go:build` tags needed (unit tests run by default)

---

## Test Cases

### Configuration Validation

These test cases validate the driver configuration parsing and validation logic. The config includes: S3 endpoint, Admin API endpoint, access key, secret key, region (optional, default: "us-east-1"), driver name (optional, default: "versitygw.cosi.dev").

#### TC-U-001: Valid config with all required fields

- **Given**: A config with s3Endpoint="https://s3.example.com", adminEndpoint="https://admin.example.com", accessKey="AKID", secretKey="SECRET"
- **When**: Validate is called
- **Then**: No error is returned

#### TC-U-002: Missing S3 endpoint

- **Given**: A config with s3Endpoint="" and all other fields valid
- **When**: Validate is called
- **Then**: Error is returned indicating S3 endpoint is required

#### TC-U-003: Missing Admin endpoint

- **Given**: A config with adminEndpoint="" and all other fields valid
- **When**: Validate is called
- **Then**: Error is returned indicating Admin endpoint is required

#### TC-U-004: Missing access key

- **Given**: A config with accessKey="" and all other fields valid
- **When**: Validate is called
- **Then**: Error is returned indicating access key is required

#### TC-U-005: Missing secret key

- **Given**: A config with secretKey="" and all other fields valid
- **When**: Validate is called
- **Then**: Error is returned indicating secret key is required

#### TC-U-006: Invalid endpoint URL format

- **Given**: A config with s3Endpoint="not-a-url"
- **When**: Validate is called
- **Then**: Error is returned indicating invalid URL format

#### TC-U-007: Default region applied

- **Given**: A config with region="" and all required fields valid
- **When**: Parse/apply defaults is called
- **Then**: Region is set to "us-east-1"

#### TC-U-008: Default driver name applied

- **Given**: A config with driverName="" and all required fields valid
- **When**: Parse/apply defaults is called
- **Then**: Driver name is set to the project's default (e.g., "versitygw.cosi.dev")

---

### Driver Name Validation

The driver name must follow domain name notation (RFC 1035 section 2.3.1), be <= 63 characters, begin and end with alphanumeric, with dashes, dots, and alphanumerics between.

#### TC-U-010: Valid domain-name format

- **Given**: name = "versitygw.cosi.dev"
- **When**: ValidateDriverName is called
- **Then**: No error

#### TC-U-011: Name exceeds 63 characters

- **Given**: name = a 64-character string of valid characters
- **When**: ValidateDriverName is called
- **Then**: Error indicating name too long

#### TC-U-012: Name with invalid characters

- **Given**: name = "versitygw_cosi" (underscore is not allowed in domain names)
- **When**: ValidateDriverName is called
- **Then**: Error indicating invalid characters

#### TC-U-013: Name not starting with alphanumeric

- **Given**: name = "-versitygw.cosi.dev"
- **When**: ValidateDriverName is called
- **Then**: Error indicating name must start with alphanumeric

#### TC-U-014: Name not ending with alphanumeric

- **Given**: name = "versitygw.cosi.dev-"
- **When**: ValidateDriverName is called
- **Then**: Error indicating name must end with alphanumeric

#### TC-U-015: Empty name

- **Given**: name = ""
- **When**: ValidateDriverName is called
- **Then**: Error indicating name is required

---

### Bucket ID Generation / Parsing

The driver must map COSI bucket names to bucket IDs. The bucket ID is what VersityGW uses as the actual S3 bucket name. Depending on the implementation, this may be the name itself, a hash, or a prefixed variant.

#### TC-U-020: Generate ID from valid bucket name

- **Given**: name = "my-test-bucket"
- **When**: BucketNameToID is called
- **Then**: Returns a non-empty string that is a valid S3 bucket name

#### TC-U-021: Same name produces same ID (deterministic)

- **Given**: name = "my-test-bucket"
- **When**: BucketNameToID is called twice
- **Then**: Both calls return identical IDs

#### TC-U-022: Different names produce different IDs

- **Given**: name1 = "bucket-a", name2 = "bucket-b"
- **When**: BucketNameToID is called for each
- **Then**: The returned IDs are different

#### TC-U-023: Empty name returns error

- **Given**: name = ""
- **When**: BucketNameToID is called
- **Then**: Returns error (empty name is not valid)

#### TC-U-024: Name at maximum length boundary

- **Given**: name = a 63-character valid DNS label
- **When**: BucketNameToID is called
- **Then**: Returns a valid ID without error

---

### Bucket Name Validation

S3 bucket names must be DNS-compatible: 3-63 characters, lowercase letters/numbers/hyphens, must start and end with letter or number, must not be formatted as an IP address.

#### TC-U-025: Valid DNS-compatible name

- **Given**: name = "my-test-bucket-123"
- **When**: ValidateBucketName is called
- **Then**: No error

#### TC-U-026: Name too short

- **Given**: name = "ab" (2 characters)
- **When**: ValidateBucketName is called
- **Then**: Error indicating name too short (minimum 3)

#### TC-U-027: Name too long

- **Given**: name = a 64-character string
- **When**: ValidateBucketName is called
- **Then**: Error indicating name too long (maximum 63)

#### TC-U-028: Name with uppercase letters

- **Given**: name = "My-Bucket"
- **When**: ValidateBucketName is called
- **Then**: Error indicating lowercase only

#### TC-U-029: Name with underscores

- **Given**: name = "my_bucket"
- **When**: ValidateBucketName is called
- **Then**: Error indicating invalid character (underscores not DNS-compatible)

#### TC-U-030: Name formatted as IP address

- **Given**: name = "192.168.1.1"
- **When**: ValidateBucketName is called
- **Then**: Error indicating bucket name must not be an IP address

---

### Bucket Policy Construction

The driver grants access by setting S3 bucket policies. VersityGW uses access key IDs directly as principals (NOT ARNs). The driver must construct valid JSON policies.

Policy structure:
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {"AWS": ["<accessKeyID1>", "<accessKeyID2>"]},
      "Action": ["s3:GetObject", "s3:PutObject", ...],
      "Resource": ["arn:aws:s3:::<bucket>", "arn:aws:s3:::<bucket>/*"]
    }
  ]
}
```

#### TC-U-040: New policy with read-write actions

- **Given**: bucketName = "test-bucket", accessKeyID = "AKID001", accessMode = ReadWrite
- **When**: NewBucketPolicy is called
- **Then**:
  - Policy Version is "2012-10-17"
  - Statement has Effect "Allow"
  - Principal.AWS contains "AKID001" (NOT an ARN)
  - Action contains at minimum: `s3:PutObject`, `s3:GetObject`, `s3:DeleteObject`, `s3:ListBucket`
  - Resource contains `arn:aws:s3:::test-bucket` and `arn:aws:s3:::test-bucket/*`

#### TC-U-041: New policy with read-only actions

- **Given**: bucketName = "test-bucket", accessKeyID = "AKID001", accessMode = ReadOnly
- **When**: NewBucketPolicy is called
- **Then**:
  - Action contains `s3:GetObject` and `s3:ListBucket`
  - Action does NOT contain `s3:PutObject` or `s3:DeleteObject`

#### TC-U-042: New policy with write-only actions

- **Given**: bucketName = "test-bucket", accessKeyID = "AKID001", accessMode = WriteOnly
- **When**: NewBucketPolicy is called
- **Then**:
  - Action contains `s3:PutObject`
  - Action does NOT contain `s3:GetObject`

#### TC-U-043: Add principal to existing policy

- **Given**: An existing policy with principal "AKID001"
- **When**: AddPrincipal is called with accessKeyID = "AKID002" and same actions
- **Then**:
  - Principal.AWS contains both "AKID001" and "AKID002"
  - Only one statement exists (merged, not duplicated)

#### TC-U-044: Remove principal from multi-principal policy

- **Given**: An existing policy with principals ["AKID001", "AKID002"]
- **When**: RemovePrincipal is called with accessKeyID = "AKID001"
- **Then**:
  - Principal.AWS contains only "AKID002"
  - Statement still exists

#### TC-U-045: Remove last principal from policy

- **Given**: An existing policy with single principal "AKID001"
- **When**: RemovePrincipal is called with accessKeyID = "AKID001"
- **Then**: Returns nil/empty policy (indicating the entire policy should be deleted)

#### TC-U-046: Principal uses access key ID, not ARN

- **Given**: bucketName = "test-bucket", accessKeyID = "AKIAIOSFODNN7EXAMPLE"
- **When**: NewBucketPolicy is called
- **Then**: Principal.AWS contains the raw access key ID string, NOT an ARN like "arn:aws:iam::..."

#### TC-U-047: Resource format is correct

- **Given**: bucketName = "my-bucket"
- **When**: NewBucketPolicy is called
- **Then**:
  - Resource list has exactly 2 entries
  - One is `arn:aws:s3:::my-bucket` (bucket-level operations like ListBucket)
  - One is `arn:aws:s3:::my-bucket/*` (object-level operations)

#### TC-U-048: Actions map correctly for each access mode (table-driven)

- **Given**: Each defined access mode (ReadOnly, WriteOnly, ReadWrite)
- **When**: ActionsForAccessMode is called
- **Then**:

| Access Mode | Must Include                                        | Must NOT Include           |
|-------------|-----------------------------------------------------|----------------------------|
| ReadOnly    | `s3:GetObject`, `s3:ListBucket`                     | `s3:PutObject`, `s3:DeleteObject` |
| WriteOnly   | `s3:PutObject`                                      | `s3:GetObject`             |
| ReadWrite   | `s3:GetObject`, `s3:PutObject`, `s3:DeleteObject`, `s3:ListBucket` | (none)        |

---

### Admin API Request Construction

VersityGW has a custom Admin API using PATCH requests with SigV4 signing (service name: "s3"). User data is serialized as XML.

#### TC-U-050: Create user request XML structure

- **Given**: accessKey = "NEWAKID", secretKey = "NEWSECRET", role = "user"
- **When**: Serialize to XML
- **Then**: XML contains `<access>NEWAKID</access>`, `<secret>NEWSECRET</secret>`, `<role>user</role>`

#### TC-U-051: Create user request includes all required fields

- **Given**: A complete user account struct
- **When**: Serialize to XML
- **Then**: The XML includes access, secret, and role fields. Optional fields (userID, groupID, projectID) may be empty.

#### TC-U-052: Delete user URL construction

- **Given**: accessKey = "AKID_TO_DELETE"
- **When**: Build delete user URL
- **Then**: URL path is `/delete-user` with query parameter `access=AKID_TO_DELETE`

#### TC-U-053: Parse list users XML response

- **Given**: A valid XML response containing 2 user accounts
- **When**: Parse list users response
- **Then**: Returns a slice of 2 user accounts with correct access, secret, and role fields

---

### Credential Mapping

The driver must return credentials in the COSI v1alpha1 format: `map<string, CredentialDetails>` where the key is the protocol name and `CredentialDetails.secrets` is a `map<string, string>`.

#### TC-U-060: Credentials map has "s3" key

- **Given**: accessKeyID = "AKID", secretKey = "SECRET", endpoint = "https://s3.example.com", region = "us-east-1"
- **When**: Map to COSI credential format
- **Then**: The result map has exactly one key: `"s3"`

#### TC-U-061: CredentialDetails contains accessKeyID

- **Given**: accessKeyID = "AKID"
- **When**: Map to COSI credential format
- **Then**: `credentials["s3"].secrets["accessKeyID"]` == "AKID"

#### TC-U-062: CredentialDetails contains accessSecretKey

- **Given**: secretKey = "SECRET"
- **When**: Map to COSI credential format
- **Then**: `credentials["s3"].secrets["accessSecretKey"]` == "SECRET"

#### TC-U-063: CredentialDetails contains endpoint

- **Given**: endpoint = "https://s3.example.com"
- **When**: Map to COSI credential format
- **Then**: `credentials["s3"].secrets["endpoint"]` == "https://s3.example.com"

#### TC-U-064: CredentialDetails contains region

- **Given**: region = "us-east-1"
- **When**: Map to COSI credential format
- **Then**: `credentials["s3"].secrets["region"]` == "us-east-1"

---

### Protocol Construction

The `DriverCreateBucketResponse` must include a `Protocol` message with the S3 oneof set.

```protobuf
message Protocol {
    oneof type {
        S3 s3 = 1;           // { region, signature_version }
        AzureBlob azureBlob = 2;
        GCS gcs = 3;
    }
}
```

#### TC-U-070: S3 protocol with region and SigV4

- **Given**: region = "us-east-1", signatureVersion = S3V4
- **When**: NewS3Protocol is called
- **Then**:
  - Protocol.S3 is non-nil
  - Protocol.S3.Region == "us-east-1"
  - Protocol.S3.SignatureVersion == S3V4

#### TC-U-071: S3 protocol with SigV2

- **Given**: region = "us-east-1", signatureVersion = S3V2
- **When**: NewS3Protocol is called
- **Then**: Protocol.S3.SignatureVersion == S3V2

#### TC-U-072: S3 protocol with empty region uses default

- **Given**: region = "", signatureVersion = S3V4
- **When**: NewS3Protocol is called
- **Then**: Protocol.S3.Region == "" or the configured default region

---

### Error Mapping

The driver translates S3 SDK errors and Admin API HTTP errors to gRPC status codes as defined by the COSI spec.

#### TC-U-080: S3 BucketAlreadyOwnedByYou -> codes.OK

- **Given**: An S3 error of type BucketAlreadyOwnedByYou
- **When**: MapS3Error is called in CreateBucket context
- **Then**: Returns nil (treated as idempotent success)

#### TC-U-081: S3 BucketAlreadyExists -> codes.AlreadyExists

- **Given**: An S3 error of type BucketAlreadyExists
- **When**: MapS3Error is called in CreateBucket context
- **Then**: Returns gRPC status with code AlreadyExists

#### TC-U-082: S3 NoSuchBucket in delete context -> codes.OK

- **Given**: An S3 error of type NoSuchBucket
- **When**: MapS3Error is called in DeleteBucket context
- **Then**: Returns nil (idempotent -- bucket already gone)

#### TC-U-083: S3 AccessDenied -> codes.PermissionDenied

- **Given**: An S3 error with HTTP 403 / AccessDenied
- **When**: MapS3Error is called
- **Then**: Returns gRPC status with code PermissionDenied

#### TC-U-084: S3 unknown error -> codes.Internal

- **Given**: An unrecognized S3 error
- **When**: MapS3Error is called
- **Then**: Returns gRPC status with code Internal, wrapping the original error message

#### TC-U-085: Admin API HTTP 401 -> codes.Unauthenticated

- **Given**: Admin API returns HTTP 401
- **When**: MapAdminError is called
- **Then**: Returns gRPC status with code Unauthenticated

#### TC-U-086: Admin API HTTP 403 -> codes.PermissionDenied

- **Given**: Admin API returns HTTP 403
- **When**: MapAdminError is called
- **Then**: Returns gRPC status with code PermissionDenied

#### TC-U-087: Admin API HTTP 409 -> codes.AlreadyExists

- **Given**: Admin API returns HTTP 409
- **When**: MapAdminError is called
- **Then**: Returns gRPC status with code AlreadyExists

#### TC-U-088: Admin API HTTP 404 -> codes.NotFound

- **Given**: Admin API returns HTTP 404
- **When**: MapAdminError is called
- **Then**: Returns gRPC status with code NotFound

#### TC-U-089: Admin API HTTP 500 -> codes.Internal

- **Given**: Admin API returns HTTP 500
- **When**: MapAdminError is called
- **Then**: Returns gRPC status with code Internal

---

### Request Validation

Each gRPC RPC has required fields. The driver must validate these and return `INVALID_ARGUMENT` before attempting any backend calls.

#### TC-U-090: CreateBucket with empty name

- **Given**: DriverCreateBucketRequest{Name: ""}
- **When**: ValidateCreateBucketRequest is called
- **Then**: Error with gRPC code INVALID_ARGUMENT, message mentions "name"

#### TC-U-091: DeleteBucket with empty bucket_id

- **Given**: DriverDeleteBucketRequest{BucketId: ""}
- **When**: ValidateDeleteBucketRequest is called
- **Then**: Error with gRPC code INVALID_ARGUMENT, message mentions "bucket_id"

#### TC-U-092: GrantAccess with empty bucket_id

- **Given**: DriverGrantBucketAccessRequest{BucketId: "", Name: "user1", AuthenticationType: Key}
- **When**: ValidateGrantAccessRequest is called
- **Then**: Error with gRPC code INVALID_ARGUMENT, message mentions "bucket_id"

#### TC-U-093: GrantAccess with empty name

- **Given**: DriverGrantBucketAccessRequest{BucketId: "bucket1", Name: "", AuthenticationType: Key}
- **When**: ValidateGrantAccessRequest is called
- **Then**: Error with gRPC code INVALID_ARGUMENT, message mentions "name"

#### TC-U-094: GrantAccess with unknown auth type (0)

- **Given**: DriverGrantBucketAccessRequest{BucketId: "bucket1", Name: "user1", AuthenticationType: UnknownAuthenticationType}
- **When**: ValidateGrantAccessRequest is called
- **Then**: Error with gRPC code INVALID_ARGUMENT, message mentions "authentication_type"

#### TC-U-095: GrantAccess with IAM auth type (unsupported by VersityGW)

- **Given**: DriverGrantBucketAccessRequest{BucketId: "bucket1", Name: "user1", AuthenticationType: IAM}
- **When**: ValidateGrantAccessRequest is called
- **Then**: Error with gRPC code INVALID_ARGUMENT, message indicates IAM authentication is not supported

> This is a critical test: VersityGW has no STS, so the driver cannot provide IAM-based authentication. The driver MUST reject this request early with a clear error message rather than failing partway through the grant flow.

#### TC-U-096: RevokeAccess with empty bucket_id

- **Given**: DriverRevokeBucketAccessRequest{BucketId: "", AccountId: "acc1"}
- **When**: ValidateRevokeAccessRequest is called
- **Then**: Error with gRPC code INVALID_ARGUMENT, message mentions "bucket_id"

#### TC-U-097: RevokeAccess with empty account_id

- **Given**: DriverRevokeBucketAccessRequest{BucketId: "bucket1", AccountId: ""}
- **When**: ValidateRevokeAccessRequest is called
- **Then**: Error with gRPC code INVALID_ARGUMENT, message mentions "account_id"

---

## Summary

| ID Range        | Area                        | Count |
|-----------------|-----------------------------|-------|
| TC-U-001 ~ 008  | Configuration validation    | 8     |
| TC-U-010 ~ 015  | Driver name validation      | 6     |
| TC-U-020 ~ 024  | Bucket ID generation        | 5     |
| TC-U-025 ~ 030  | Bucket name validation      | 6     |
| TC-U-040 ~ 048  | Bucket policy construction  | 9     |
| TC-U-050 ~ 053  | Admin API request building  | 4     |
| TC-U-060 ~ 064  | Credential mapping          | 5     |
| TC-U-070 ~ 072  | Protocol construction       | 3     |
| TC-U-080 ~ 089  | Error mapping               | 10    |
| TC-U-090 ~ 097  | Request validation          | 8     |
| **Total**       |                             | **64**|
