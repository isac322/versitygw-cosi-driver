# Changelog

All notable changes to versitygw-cosi-driver will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.4.0] - 2026-04-21

### Added

- `.github/workflows/release-verifier.yaml` builds and pushes the Chainsaw e2e
  verifier image to GHCR (`ghcr.io/isac322/versitygw-cosi-driver/verifier`) on
  master changes under `test/chainsaw/verifier/**`. `setup.sh` pulls the
  resulting `:latest` tag, saving ~35s per e2e run over rebuilding locally.
- `idle` mode (and default no-op first-arg behavior) in the verifier
  `entrypoint.sh` that runs `sleep infinity`, enabling recovery tests to
  provision one long-lived verifier Pod and drive S3 checks via
  `kubectl exec` instead of spawning a fresh Job per verification.
- `test/chainsaw/bootstrap/cosi-controller-regen.sh` script to regenerate
  the pre-rendered `cosi-v<version>-crds.yaml` /
  `cosi-v<version>-controller.yaml` manifests used by the e2e install
  path when bumping COSI versions.
- `make test-e2e-main-only` and `make test-e2e-recovery-only` Makefile
  targets that provision their own kind cluster (`versitygw-cosi-chainsaw-main`
  / `-recovery`) with independent project-local kubeconfigs
  (`.e2e-kubeconfig-main`, `.e2e-kubeconfig-recovery`). Enables running
  either suite in isolation and backs the parallel `make test-e2e-all`.
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
- `make test-e2e-all` now dumps cluster diagnostics (pods, events, jobs,
  failing-pod describes, chainsaw-namespace pod logs with `--previous`, and
  driver/controller/versitygw logs) before teardown when tests fail, so
  transient CI failures can be root-caused from cluster state that would
  otherwise be lost. New `make test-e2e-diagnose` target wraps the script.
- Chainsaw `--parallel` is now driven by the `CHAINSAW_PARALLEL` Makefile
  variable (defaults to `nproc` capped at 2). The cap is deliberately low:
  local runs at parallel 8 tripped three tests on BucketAccess status
  asserts (tc-e-008/044/060), parallel 4 still tripped tc-e-060 on a
  re-run, and parallel 2 is the highest value observed reliably green
  against COSI v0.2.2's optimistic-concurrency races (upstream #79/#227).
  Smaller hosts fall through to their `nproc` value; revisit once the
  upstream controller improves.
- Chainsaw non-assert timeouts tightened: delete/error/exec 2m → 60s,
  cleanup 3m → 90s. `assert` stays at 3m because the recovery tests'
  post-restart `new-provisioning-works-after-restart` step reconciles
  in 120–130s locally (cold-path COSI reconcile after driver restart).
- `cleanup.sh` `MIN_ITERS` reduced from 5 to 2 (real finalizer-stranding
  cases already loop on their own; 5 was adding ~3s × 25 tests of idle
  work) and recovery-mode `kubectl rollout status --timeout` 60s → 30s.
- Replaced fixed sleep-and-poll loops in `tc-e-002`, `tc-e-009`, and
  `tc-e-010` with Chainsaw assert/error retries backed by the new bounded
  timeouts. Happy path returns as soon as the state is reached instead of
  burning a worst-case sleep budget.
- Recovery `tc-e-050-driver-restart` now provisions a single long-running
  verifier Pod and drives PutObject/GetObject checks via `kubectl exec`,
  replacing three per-verification Jobs. Saves ~40s per run on the
  Pod-scheduling + Job-status-reflection overhead.
- Recovery `kubectl rollout status --timeout` tightened from 120s to 60s
  (happy path is 10–20s), and recovery asserts given explicit 30s timeouts.
- E2E driver image build in CI uses `docker buildx --cache-to=type=gha`
  (via the existing Buildx builder added in the e2e job), saving ~45s on
  cache hit. Local setup.sh falls back to plain `docker build` when no
  Buildx builder is configured.
- E2E setup now pulls the prebuilt verifier image from GHCR by default
  (`ghcr.io/isac322/versitygw-cosi-driver/verifier:latest`), saving ~35s
  vs rebuilding per run. Override with `VERIFIER_SRC=build` to force a
  local rebuild; the pull path also falls back to a local build if the
  image is unavailable.
- COSI v0.2.2 CRDs and controller manifests are now pre-rendered and
  committed under `test/chainsaw/bootstrap/cosi-v0.2.2-*.yaml`; the
  install script applies them directly instead of doing `curl × 5 +
  kubectl kustomize` on every run. Regeneration lives in
  `cosi-controller-regen.sh` for version bumps.
- CI e2e job `timeout-minutes` lowered from 30 to 8; a stuck job now
  surfaces within minutes instead of burning the previous ceiling.
- Split the e2e suite across two kind clusters so the main tests and the
  recovery tests run in parallel. Locally `make test-e2e-all` uses
  `make -j2 test-e2e-main-only test-e2e-recovery-only`; in CI the single
  `e2e` job is replaced by `e2e-main` (5m timeout) and `e2e-recovery`
  (7m timeout) on independent runners. Locally measured: 335s wallclock
  (5m 35s) vs 437s if run back-to-back. CI is expected at ~5m critical
  path (main ~4m, recovery ~5m, parallel) vs ~8m single-job baseline.
  `setup.sh` now deletes any stale cluster with the same name before
  creating, making the target safe to re-run after crashes.

### Removed

- Legacy Go E2E framework (`test/e2e/*.go`, 2262 lines based on
  `sigs.k8s.io/e2e-framework`) in favor of the Chainsaw suite. `go.mod` no
  longer depends on `sigs.k8s.io/e2e-framework`, `k8s.io/api`, or
  `k8s.io/apimachinery`.
- Redundant "Collect cluster diagnostics on failure" step from the CI
  workflow; it ran after `test-e2e-all`'s teardown and produced only
  "connection refused" output. Diagnostics now live in `test/chainsaw/diagnose.sh`
  and run before teardown via the Makefile.

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

[Unreleased]: https://github.com/isac322/versitygw-cosi-driver/compare/app-v0.4.0...HEAD
[0.4.0]: https://github.com/isac322/versitygw-cosi-driver/compare/app-v0.3.0...app-v0.4.0
[0.3.0]: https://github.com/isac322/versitygw-cosi-driver/compare/app-v0.2.0...app-v0.3.0
[0.2.0]: https://github.com/isac322/versitygw-cosi-driver/compare/app-v0.1.0...app-v0.2.0
[0.1.0]: https://github.com/isac322/versitygw-cosi-driver/releases/tag/app-v0.1.0
