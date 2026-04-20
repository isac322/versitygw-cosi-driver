# Changelog

All notable changes to versitygw-cosi-driver will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `internal/config` package with `Config` struct, `Validate`, and `ApplyDefaults`
  for testable configuration validation (extracted from inline main.go logic).
- Test pyramid documentation under `docs/tests/` defining unit, component,
  integration, and E2E test strategies with TC-U/TC-C/TC-I/TC-E identifiers.
- Unit tests covering config validation, driver name behavior, bucket ID
  generation, credential mapping, protocol construction, and request validation.
- Component tests using real gRPC server + mocked S3/Admin HTTP servers to
  verify RPC contract, idempotency, error codes, and cleanup behavior.
- Integration tests covering S3/Admin API interactions with real VersityGW
  process, including IAM auth rejection, single-user mode detection, and
  connectivity failures.
- E2E test suite using Kyverno Chainsaw (`test/chainsaw/`), aligning with
  the upstream COSI conformance testing pattern. Covers 27 test cases across
  lifecycle, secret validation, Pod consumption, error, recovery, and
  multi-access scenarios. In-cluster verifier image bundles `amazon/aws-cli`
  + `jq` for declarative S3 assertions.
- `make test-e2e`, `make test-e2e-setup`, `make test-e2e-teardown`,
  `make test-e2e-recovery`, `make test-e2e-all`, `make test-e2e-keep`, and
  `make install-chainsaw` Makefile targets.
- CONTRIBUTING.md, SECURITY.md, and CODE_OF_CONDUCT.md for community guidelines.
- Helm chart keywords and enriched description for Artifact Hub / `helm search`
  discoverability.

### Changed

- Bumped `github.com/versity/versitygw` from v1.3.1 to v1.4.0. No API changes in
  the `auth` package, admin API routes, or CLI flags consumed by this driver.
- Pinned versitygw version to 1.4.0 in integration tests
  (`integration/testmain_test.go`) and Chainsaw E2E bootstrap
  (`test/chainsaw/bootstrap/versitygw.yaml`), replacing previous `@latest` /
  `latest` references for reproducible CI runs.
- Fixed VersityGW compatibility entry in `README.md` from `0.x` to `1.4.x`.
- `DriverCreateBucket` now rejects unsupported `parameters` in the request with
  `INVALID_ARGUMENT`. The driver does not accept any parameters.
- Refactored `main.go` to use `internal/config.Config` struct for flag binding and
  validation instead of inline variable declarations and ad-hoc checks.
- `testutil.StartVersityGW` readiness timeout increased from 10s to 30s to reduce
  flakiness when many instances start in parallel.
- README.md overhauled for promotion readiness: added "Why?" section, compatibility
  matrix, end-to-end Quick Start with kubectl output, Mermaid architecture diagram,
  troubleshooting guide, alternatives comparison, and Kustomize install instructions.
- Helm Quick Start now uses OCI registry (`oci://ghcr.io/...`) instead of local path.

### Removed

- Legacy Go E2E framework (`test/e2e/*.go`, 2262 lines based on
  `sigs.k8s.io/e2e-framework`) in favor of the Chainsaw suite. `go.mod` no
  longer depends on `sigs.k8s.io/e2e-framework`, `k8s.io/api`, or
  `k8s.io/apimachinery`.

## [0.3.0] - 2026-03-16

### Added

- `--driver-name` CLI flag and `DRIVER_NAME` environment variable for configurable
  COSI driver name. The driver name is now a required parameter.
- Kustomize deployment support as an alternative to Helm. Base manifests include
  Deployment, ServiceAccount, ClusterRole, and ClusterRoleBinding. BucketClass and
  BucketAccessClass are available as optional kustomize components. A `default` overlay
  combines base with all components for quick setup.

### Changed

- Driver name is no longer hardcoded to `versitygw.cosi.dev`. Users must explicitly
  set the driver name via CLI flag, environment variable, or Helm `driver.name` value.

## [0.2.0] - 2026-03-16

### Added

- **Multi-user bucket policy support**: `PutBucketPolicy` now merges principals into
  existing policies instead of overwriting. Granting access to a second user no longer
  removes the first user's access.
- `GetBucketPolicy` method for retrieving and parsing bucket policies from S3.
- `RemoveBucketPolicyPrincipal` method for removing a single principal from a bucket
  policy without affecting other users. Automatically deletes the policy when the last
  principal is removed.
- `policyPrincipal` type with custom JSON unmarshaling that normalizes both single-string
  (`{"AWS": "user"}`) and array (`{"AWS": ["user1", "user2"]}`) S3 principal formats.
- Proper gRPC error codes for all provisioner operations via `mapToGRPCError` helper.
  The COSI controller can now distinguish between `AlreadyExists`, `NotFound`,
  `InvalidArgument`, `FailedPrecondition`, `Canceled`, `DeadlineExceeded`, and `Internal` errors.
- Bucket name validation in `DriverCreateBucket` enforcing S3 naming rules (3-63 chars,
  lowercase alphanumeric and hyphens, must start/end with alphanumeric). Returns
  `codes.InvalidArgument` for invalid names.
- Helm template helpers `bucketClassName` and `bucketAccessClassName` with short default
  name `versitygw` instead of release-name-based names.
- VersityGW prerequisite documentation in Helm NOTES.txt (IAM and Admin API requirements).
- `sidecar.extraArgs` support in Helm chart for passing additional flags to the
  objectstorage-sidecar container (e.g., `["--v=5"]` for verbose logging).
- Unit tests for `policyPrincipal` JSON marshaling/unmarshaling (12 cases including
  edge cases for invalid input).
- Unit tests for `validateBucketName` (21 cases covering boundary lengths, invalid
  characters, and start/end constraints).
- Unit tests for `mapToGRPCError` (13 cases covering all mapped error types including
  smithy API errors, auth errors, and context errors).
- Integration tests for multi-user bucket policy: merge, duplicate principal idempotency,
  single principal removal, last principal removal, and no-policy bucket handling.
- Integration tests for driver-level multi-user grant and selective revoke.

### Changed

- `DriverRevokeBucketAccess` now removes only the specified user's principal from the
  bucket policy instead of deleting the entire policy. Other users' access is preserved.
- `BucketClass` and `BucketAccessClass` default names changed from
  `<release-name>-versitygw-cosi-driver` to `versitygw`.
- Example manifests updated to use the new default class name `versitygw`.

### Fixed

- Revoking one user's bucket access no longer removes all other users' access to the
  same bucket.
- gRPC errors now carry proper status codes instead of `codes.Unknown`, enabling the
  COSI controller to handle errors appropriately (e.g., retry vs. fail-fast decisions).

## [0.1.0] - 2025-05-01

### Added

- Initial release of versitygw-cosi-driver.
- COSI `DriverCreateBucket` / `DriverDeleteBucket` with idempotent handling.
- COSI `DriverGrantBucketAccess` / `DriverRevokeBucketAccess` with per-user S3 credentials.
- VersityGW Admin API integration for user management (create, delete, list).
- Bucket policy management for granting S3 access.
- Helm chart with configurable VersityGW endpoints, credentials, RBAC, and sidecar.
- Integration test suite with embedded versitygw process.
- GitHub Actions CI/CD pipeline with container image and Helm chart releases.
- Dockerfile with multi-stage build and security hardening.

[Unreleased]: https://github.com/isac322/versitygw-cosi-driver/compare/app-v0.3.0...HEAD
[0.3.0]: https://github.com/isac322/versitygw-cosi-driver/compare/app-v0.2.0...app-v0.3.0
[0.2.0]: https://github.com/isac322/versitygw-cosi-driver/compare/app-v0.1.0...app-v0.2.0
[0.1.0]: https://github.com/isac322/versitygw-cosi-driver/releases/tag/app-v0.1.0
