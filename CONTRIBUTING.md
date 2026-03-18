# Contributing to versitygw-cosi-driver

Thank you for your interest in contributing! This guide will help you get started.

## Development Setup

### Prerequisites

- Go 1.24+
- [golangci-lint](https://golangci-lint.run/welcome/install/)
- [Helm](https://helm.sh/docs/intro/install/) (for chart testing)
- [VersityGW](https://github.com/versity/versitygw) binary in PATH (for integration tests)

### Building

```bash
make build
```

### Running Tests

```bash
# Unit tests
make test

# Integration tests (requires versitygw binary)
make integration-test

# Lint
make lint
```

## Code Guidelines

- **Tests required**: All code changes must include corresponding tests.
- **CHANGELOG**: Update the `[Unreleased]` section in `CHANGELOG.md` for every change, following [Keep a Changelog](https://keepachangelog.com) format.
- **Linting**: All code must pass `golangci-lint`. Key rules:
  - `t.Parallel()` required in all tests (tparallel)
  - `t.Helper()` required in test helpers (thelper)
  - Use `testify/require` (not `assert`) for test assertions
- **Error wrapping**: Use `fmt.Errorf("context: %w", err)` to wrap errors.

## Pull Request Process

1. Fork the repository and create a feature branch from `master`
2. Make your changes with tests and CHANGELOG updates
3. Ensure `make lint` and `make test` pass
4. Open a PR with a clear description of the change and its motivation
5. Address review feedback

## Reporting Issues

Open an issue on [GitHub Issues](https://github.com/isac322/versitygw-cosi-driver/issues) with:

- A clear description of the problem or feature request
- Steps to reproduce (for bugs)
- Expected vs. actual behavior
- Driver version, Kubernetes version, and VersityGW version
