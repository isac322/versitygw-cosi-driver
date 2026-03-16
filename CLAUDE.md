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

## Release

- **App**: `gh release create app-v<version>` → triggers Docker image build/push to GHCR (`release-app.yaml`)
- **Chart**: bump `version` and `appVersion` in `deploy/helm/versitygw-cosi-driver/Chart.yaml` and push to master → auto-creates `chart-v<version>` tag and releases to GHCR OCI registry (`release-chart.yaml`)
- Update `CHANGELOG.md` Unreleased → `[<version>] - <date>` before releasing
- App and Chart are released independently. Do not modify files for one when releasing the other
- All code, comments, documentation, commit messages, PR bodies, and release notes in this project MUST be written in **English**

## Gotchas

- `integration/` directory is exempt from wrapcheck and noctx linters
- `_test.go` files are exempt from wrapcheck
- Build uses CGO_ENABLED=0 (static binary)
