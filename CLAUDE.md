# versitygw-cosi-driver

Kubernetes COSI driver for VersityGW S3-compatible object storage.

## Testing

- `t.Parallel()` required in all tests — tparallel linter enforced
- `t.Helper()` in test helpers — thelper linter enforced
- Use `testify/require` (not `assert`) for assertions
- Integration tests use `testutil.StartVersityGW(t)` for per-test VersityGW instances

## Workflow

- Always write corresponding tests when modifying or adding code
- Always update `CHANGELOG.md` in [Keep a Changelog](https://keepachangelog.com) format when changing code (add to Unreleased section)

## Gotchas

- `integration/` directory is exempt from wrapcheck and noctx linters
- `_test.go` files are exempt from wrapcheck
- Build uses CGO_ENABLED=0 (static binary)
