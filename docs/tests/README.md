# Test Strategy for versitygw-cosi-driver

## Purpose

This document defines the test strategy for the VersityGW COSI driver. The strategy is designed to achieve three coverage goals:

1. **COSI spec resilience** -- Detect breaking changes when the COSI specification evolves (e.g., v1alpha1 -> v1alpha2). Unit and component tests validate the gRPC contract (field formats, error codes, idempotency semantics).
2. **VersityGW version compatibility** -- Detect regressions when VersityGW upgrades change S3 or Admin API behavior. Integration tests run against a real VersityGW instance and should be executed across a version matrix in CI.
3. **Kubernetes version compatibility** -- Detect issues when Kubernetes or the COSI controller/sidecar evolves. E2E tests run in a real cluster with the full COSI stack.

Additionally, every layer provides regression safety for new features and bug fixes.

## Test Pyramid

```
            /\
           /E2E\           Full K8s + COSI controller + sidecar + VersityGW
          /------\          Validates: CRD lifecycle, Secret creation, Pod consumption
         /Integr- \         Real gRPC server + real VersityGW (no K8s)
        / ation    \        Validates: S3 ops, Admin API, credential usability, policy correctness
       /------------\
      / Component    \      Real gRPC server + mocked S3/Admin clients
     /                \     Validates: RPC contract, idempotency, error codes, cleanup
    /------------------\
   /       Unit         \   Pure functions, no I/O
  /                      \  Validates: config, policy JSON, XML, error mapping, validation
 /________________________\
```

| Layer       | Speed    | External Deps         | What It Catches                              |
|-------------|----------|-----------------------|----------------------------------------------|
| Unit        | < 1s     | None                  | Logic bugs, format errors, mapping mistakes  |
| Component   | < 5s     | None (mocked)         | RPC contract violations, error handling gaps  |
| Integration | < 60s    | VersityGW process     | S3/Admin API incompatibilities, real I/O bugs |
| E2E         | < 10min  | K8s cluster + VersityGW| Full-stack workflow failures                 |

## COSI Spec Version

This project implements **COSI v1alpha1** (`sigs.k8s.io/container-object-storage-interface-spec v0.1.0`).

### RPCs Under Test

| RPC                        | Unit | Component | Integration | E2E |
|----------------------------|------|-----------|-------------|-----|
| Identity.DriverGetInfo     | Yes  | Yes       | Yes         | Yes |
| Provisioner.DriverCreateBucket    | Yes  | Yes       | Yes         | Yes |
| Provisioner.DriverDeleteBucket    | Yes  | Yes       | Yes         | Yes |
| Provisioner.DriverGrantBucketAccess  | Yes  | Yes       | Yes         | Yes |
| Provisioner.DriverRevokeBucketAccess | Yes  | Yes       | Yes         | Yes |

### VersityGW Interactions Under Test

| Operation                     | Layer            | VersityGW API Used              |
|-------------------------------|------------------|---------------------------------|
| Create S3 bucket              | Integration, E2E | `S3 CreateBucket`               |
| Delete S3 bucket              | Integration, E2E | `S3 DeleteBucket`               |
| Check bucket existence        | Integration, E2E | `S3 HeadBucket`                 |
| Create IAM user               | Integration, E2E | `Admin PATCH /create-user`      |
| Delete IAM user               | Integration, E2E | `Admin PATCH /delete-user`      |
| List IAM users                | Integration, E2E | `Admin PATCH /list-users`       |
| Set bucket policy             | Integration, E2E | `S3 PutBucketPolicy`            |
| Get bucket policy             | Integration, E2E | `S3 GetBucketPolicy`            |
| Delete bucket policy          | Integration, E2E | `S3 DeleteBucketPolicy`         |
| S3 object operations (verify) | Integration, E2E | `S3 PutObject/GetObject/etc.`   |

### Unsupported COSI Features (Must Return Proper Errors)

| Feature                          | Reason                        | Expected Error       | Tested At     |
|----------------------------------|-------------------------------|----------------------|---------------|
| `AuthenticationType.IAM`        | VersityGW has no STS          | `INVALID_ARGUMENT`   | Unit, Component, Integration, E2E |
| Azure/GCS protocol               | VersityGW is S3-only          | N/A (driver never called) | Component (defensive) |
| `AuthenticationType` unknown (0) | Invalid enum value            | `INVALID_ARGUMENT`   | Unit, Component |

### COSI Spec gRPC Error Code Coverage

| gRPC Code         | Scenario                                  | Tested At          |
|-------------------|-------------------------------------------|--------------------|
| OK                | Success + idempotent success              | All layers         |
| INVALID_ARGUMENT  | Missing/invalid fields, unsupported auth  | Unit, Component    |
| ALREADY_EXISTS    | Name conflict with different params       | Unit, Component    |
| PERMISSION_DENIED | S3 403, Admin 403                         | Unit, Component    |
| ABORTED           | Concurrent ops on same resource           | Component (TC-C-063) |
| UNAUTHENTICATED   | Bad credentials (S3 401, Admin 401)       | Unit, Component    |
| INTERNAL          | Unknown backend errors                    | Unit, Component    |

### COSI Spec Response Requirements

| Requirement                                  | Tested At              |
|----------------------------------------------|------------------------|
| Error message MUST be human-readable         | Component (TC-C-065)   |
| Error details MUST be empty                  | Component (TC-C-066)   |
| Unix socket transport (COSI_ENDPOINT)        | Integration (TC-I-065) |
| bucket_id globally unique                    | Unit (TC-U-022)        |
| Protocol.S3 set in CreateBucket response     | Component, Integration |
| Credentials map has "s3" key                 | Unit, Component, Integration |

## Go Testing Conventions

All test code in this project MUST follow these conventions:

- **`t.Parallel()`** in every test function and subtest -- enforced by `tparallel` linter
- **`t.Helper()`** in every test helper function -- enforced by `thelper` linter
- **`testify/require`** for assertions (not `assert`) -- fail fast on first assertion failure
- **Table-driven tests** preferred for parameterized scenarios
- **Test function naming**: `Test<Function>_<Scenario>` or `Test<RPC>_<Scenario>`
- **Subtests**: `t.Run("<scenario>", func(t *testing.T) { ... })`
- **Integration tests**: use build tag `//go:build integration` or `testutil.StartVersityGW(t)` for per-test instances
- **E2E tests**: declarative Kyverno Chainsaw YAML under `test/chainsaw/tests/`
  (and `test/chainsaw/recovery/` for serial recovery tests). See
  [e2e-tests.md](e2e-tests.md).

## How to Use These Documents

Each document under `docs/tests/` specifies test cases for one pyramid layer. The documents are designed so that an agent reading a single layer document can write complete, correct test code for that layer without needing additional context.

Each document contains:
1. **Purpose** -- why this layer exists and what it validates
2. **Scope & Boundaries** -- what is real vs mocked, what is in/out of scope
3. **Architecture** -- interfaces, setup patterns, directory structure
4. **Test Cases** -- organized by feature, each with Given/When/Then
5. **Go Implementation Notes** -- specific patterns and conventions

Documents:
- [Unit Tests](unit-tests.md) -- `TC-U-*` identifiers
- [Component Tests](component-tests.md) -- `TC-C-*` identifiers
- [Integration Tests](integration-tests.md) -- `TC-I-*` identifiers
- [E2E Tests](e2e-tests.md) -- `TC-E-*` identifiers
