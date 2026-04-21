.PHONY: build test integration-test docker-build clean lint lint-fix \
        install-chainsaw test-e2e-setup test-e2e test-e2e-recovery \
        test-e2e-teardown test-e2e-diagnose test-e2e-all test-e2e-keep \
        test-e2e-main-only test-e2e-recovery-only

BINARY  := versitygw-cosi-driver
IMAGE   := versitygw-cosi-driver:latest

CHAINSAW_VERSION := v0.2.14
CHAINSAW_URL := https://github.com/kyverno/chainsaw/releases/download/$(CHAINSAW_VERSION)/chainsaw_linux_amd64.tar.gz

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/$(BINARY) ./cmd/versitygw-cosi-driver

test:
	go test ./internal/...

integration-test:
	go test -v -count=1 -timeout 300s ./integration/...

docker-build:
	docker build -t $(IMAGE) .

clean:
	rm -rf bin/

lint:
	golangci-lint run ./...

lint-fix:
	golangci-lint run --fix ./...

## E2E (Chainsaw) targets
##
## DOCKER_HOST: on Arch/Manjaro with rootless Docker, export
##   DOCKER_HOST=tcp://localhost:2375 before running. On GitHub runners the
##   default unix socket works, so leave DOCKER_HOST unset.

install-chainsaw:
	@command -v chainsaw >/dev/null 2>&1 || { \
	  echo "Installing chainsaw $(CHAINSAW_VERSION) to $$HOME/.local/bin/chainsaw..." ; \
	  mkdir -p $$HOME/.local/bin ; \
	  curl -fsSL $(CHAINSAW_URL) | tar -xz -C $$HOME/.local/bin chainsaw ; \
	  echo "Installed. Make sure $$HOME/.local/bin is on your PATH." ; \
	}

# The main suite (25 tests) and the recovery suite (2 tests that restart
# shared infrastructure) each run on their own kind cluster, so locally
# `make test-e2e-all` parallelises them with `make -j2` and in CI they
# map to two independent runners (`e2e-main`, `e2e-recovery`). Splitting
# clusters avoids recovery's rollout windows colliding with main's parallel
# reconciles. Each cluster gets its own project-local kubeconfig so neither
# suite ever touches $HOME/.kube/config.
E2E_MAIN_CLUSTER := versitygw-cosi-chainsaw-main
E2E_MAIN_KUBECONFIG := $(CURDIR)/.e2e-kubeconfig-main
E2E_RECOVERY_CLUSTER := versitygw-cosi-chainsaw-recovery
E2E_RECOVERY_KUBECONFIG := $(CURDIR)/.e2e-kubeconfig-recovery

# Legacy single-cluster targets (test-e2e-setup / test-e2e / test-e2e-recovery
# / test-e2e-teardown / test-e2e-diagnose) default to the main cluster.
# Callers can override both variables together to point them elsewhere.
KIND_CLUSTER ?= $(E2E_MAIN_CLUSTER)
E2E_KUBECONFIG ?= $(E2E_MAIN_KUBECONFIG)

test-e2e-setup:
	KIND_CLUSTER=$(KIND_CLUSTER) KUBECONFIG=$(E2E_KUBECONFIG) \
	    ./test/chainsaw/setup.sh

# CHAINSAW_PARALLEL defaults to nproc capped at 2. COSI controller v0.2.2
# has optimistic-concurrency races when reconciling multiple BucketClaim/
# BucketAccess objects in parallel ("Operation cannot be fulfilled ... the
# object has been modified" loops, upstream #79/#227). Empirical runs on a
# 24-core workstation: parallel 8 pushed three tests past the
# BucketAccess status-ready assertion, parallel 4 still tripped tc-e-060
# (multiple-bucketaccess-same-claim) on one run; parallel 2 is the highest
# value we observe as reliably green. Single-core hosts fall through to
# their nproc value. Raise the cap once the upstream controller stops
# fighting itself on concurrent BucketAccess reconciles.
CHAINSAW_PARALLEL ?= $(shell n=$$(nproc 2>/dev/null || echo 2); if [ $$n -gt 2 ]; then echo 2; else echo $$n; fi)

test-e2e: install-chainsaw
	KUBECONFIG=$(E2E_KUBECONFIG) chainsaw test test/chainsaw/tests \
	    --config test/chainsaw/chainsaw-config.yaml \
	    --values test/chainsaw/values.yaml \
	    --parallel $(CHAINSAW_PARALLEL) --full-name --skip-delete

test-e2e-recovery: install-chainsaw
	KUBECONFIG=$(E2E_KUBECONFIG) chainsaw test test/chainsaw/recovery \
	    --config test/chainsaw/chainsaw-config.yaml \
	    --values test/chainsaw/values.yaml \
	    --parallel 1 --full-name --skip-delete

test-e2e-teardown:
	KIND_CLUSTER=$(KIND_CLUSTER) KUBECONFIG=$(E2E_KUBECONFIG) \
	    ./test/chainsaw/teardown.sh

test-e2e-diagnose:
	KUBECONFIG=$(E2E_KUBECONFIG) ./test/chainsaw/diagnose.sh

# Main suite on its own kind cluster (setup → chainsaw tests → diagnose on
# failure → teardown). Runs stand-alone and as half of test-e2e-all.
# Setup + tests are grouped so the teardown below always runs, even if
# cluster provisioning itself fails — prevents stale kind clusters from
# leaking across consecutive runs.
test-e2e-main-only: install-chainsaw
	{ KIND_CLUSTER=$(E2E_MAIN_CLUSTER) KUBECONFIG=$(E2E_MAIN_KUBECONFIG) \
	      ./test/chainsaw/setup.sh && \
	  KUBECONFIG=$(E2E_MAIN_KUBECONFIG) chainsaw test test/chainsaw/tests \
	      --config test/chainsaw/chainsaw-config.yaml \
	      --values test/chainsaw/values.yaml \
	      --parallel $(CHAINSAW_PARALLEL) --full-name --skip-delete ; } ; rc=$$? ; \
	if [ $$rc -ne 0 ]; then \
	  KUBECONFIG=$(E2E_MAIN_KUBECONFIG) ./test/chainsaw/diagnose.sh || true ; \
	fi ; \
	KIND_CLUSTER=$(E2E_MAIN_CLUSTER) KUBECONFIG=$(E2E_MAIN_KUBECONFIG) \
	    ./test/chainsaw/teardown.sh || true ; \
	exit $$rc

# Recovery suite on its own kind cluster. Same shape as test-e2e-main-only.
test-e2e-recovery-only: install-chainsaw
	{ KIND_CLUSTER=$(E2E_RECOVERY_CLUSTER) KUBECONFIG=$(E2E_RECOVERY_KUBECONFIG) \
	      ./test/chainsaw/setup.sh && \
	  KUBECONFIG=$(E2E_RECOVERY_KUBECONFIG) chainsaw test test/chainsaw/recovery \
	      --config test/chainsaw/chainsaw-config.yaml \
	      --values test/chainsaw/values.yaml \
	      --parallel 1 --full-name --skip-delete ; } ; rc=$$? ; \
	if [ $$rc -ne 0 ]; then \
	  KUBECONFIG=$(E2E_RECOVERY_KUBECONFIG) ./test/chainsaw/diagnose.sh || true ; \
	fi ; \
	KIND_CLUSTER=$(E2E_RECOVERY_CLUSTER) KUBECONFIG=$(E2E_RECOVERY_KUBECONFIG) \
	    ./test/chainsaw/teardown.sh || true ; \
	exit $$rc

# test-e2e-all runs the two suites in parallel on separate kind clusters.
# Locally this halves wall-clock to ~max(main, recovery); in CI the two
# halves map to independent `e2e-main` / `e2e-recovery` jobs so -j is a no-op.
# Output from the two jobs interleaves — grep by the cluster name
# (versitygw-cosi-chainsaw-main vs -recovery) or the test name to
# disentangle a specific suite's log.
test-e2e-all:
	$(MAKE) -j2 test-e2e-main-only test-e2e-recovery-only

# Retains the main cluster for manual inspection after the main suite runs.
# Recovery suite is intentionally not included here — it restarts shared
# infrastructure, so post-mortem inspection is less useful.
test-e2e-keep:
	KIND_CLUSTER=$(E2E_MAIN_CLUSTER) KUBECONFIG=$(E2E_MAIN_KUBECONFIG) \
	    ./test/chainsaw/setup.sh
	KUBECONFIG=$(E2E_MAIN_KUBECONFIG) chainsaw test test/chainsaw/tests \
	    --config test/chainsaw/chainsaw-config.yaml \
	    --values test/chainsaw/values.yaml \
	    --parallel $(CHAINSAW_PARALLEL) --full-name --skip-delete || true
	@echo "Main cluster retained for inspection."
	@echo "  export KUBECONFIG=$(E2E_MAIN_KUBECONFIG)"
	@echo "Tear down with:"
	@echo "  make test-e2e-teardown KIND_CLUSTER=$(E2E_MAIN_CLUSTER) E2E_KUBECONFIG=$(E2E_MAIN_KUBECONFIG)"
