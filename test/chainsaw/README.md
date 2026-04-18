# E2E tests (Kyverno Chainsaw)

Declarative end-to-end tests for the VersityGW COSI driver, structured around the same
27 test cases (TC-E-001…TC-E-062) defined in `docs/tests/e2e-tests.md`. See
`docs/plans/chainsaw-migration-plan.md` for the design rationale.

## Requirements

- `docker` (reachable via the local daemon or `DOCKER_HOST`)
- `kind`
- `kubectl`
- `helm`
- `curl`
- `chainsaw` v0.2.14 (installed automatically by `make install-chainsaw`)

On Arch/Manjaro with rootless Docker:

```bash
export DOCKER_HOST=tcp://localhost:2375
```

## Running

```bash
# One-shot (setup + parallel suite + recovery suite + teardown)
make test-e2e-all

# Iterative local dev loop
make test-e2e-setup     # kind cluster + COSI + VersityGW + driver + verifier image
make test-e2e           # parallel suite (25 tests)
make test-e2e-recovery  # serial recovery suite (2 tests)
make test-e2e-teardown  # delete cluster

# Debug: keep the cluster after failures
make test-e2e-keep
# ... inspect with kubectl ...
make test-e2e-teardown
```

## Layout

```
test/chainsaw/
├── chainsaw-config.yaml       # Chainsaw execution config (timeouts, parallelism)
├── values.yaml                # Shared bindings (driverName, endpoints, image names)
├── kind-config.yaml           # kind cluster definition
├── bootstrap/                 # One-time setup manifests
│   ├── cosi-controller-install.sh
│   ├── versitygw.yaml
│   └── bucketclass-*.yaml / bucketaccessclass-*.yaml
├── verifier/                  # Custom Pod verifier image (aws-cli + jq)
│   ├── Dockerfile
│   ├── entrypoint.sh          # user-mode ops (head-bucket, put-object, ...)
│   └── admin.sh               # admin-mode ops (list-users, head-bucket-by-name, ...)
├── tests/                     # Parallel-safe test cases (25)
│   ├── tc-e-001-bucketclass/chainsaw-test.yaml
│   └── ...
├── recovery/                  # Serial-only test cases (2)
│   ├── tc-e-050-driver-restart/
│   └── tc-e-051-cosi-controller-restart/
├── setup.sh
└── teardown.sh
```
