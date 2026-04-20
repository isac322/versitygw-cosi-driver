.PHONY: build test integration-test docker-build clean lint lint-fix \
        install-chainsaw test-e2e-setup test-e2e test-e2e-recovery \
        test-e2e-teardown test-e2e-diagnose test-e2e-all test-e2e-keep

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

# E2E targets export KUBECONFIG pointing at a project-local file written
# by setup.sh, so kind/kubectl/helm/chainsaw never touch $HOME/.kube/config.
E2E_KUBECONFIG := $(CURDIR)/.e2e-kubeconfig

test-e2e-setup:
	./test/chainsaw/setup.sh

test-e2e: install-chainsaw
	KUBECONFIG=$(E2E_KUBECONFIG) chainsaw test test/chainsaw/tests \
	    --config test/chainsaw/chainsaw-config.yaml \
	    --values test/chainsaw/values.yaml \
	    --parallel 4 --full-name --skip-delete

test-e2e-recovery: install-chainsaw
	KUBECONFIG=$(E2E_KUBECONFIG) chainsaw test test/chainsaw/recovery \
	    --config test/chainsaw/chainsaw-config.yaml \
	    --values test/chainsaw/values.yaml \
	    --parallel 1 --full-name --skip-delete

test-e2e-teardown:
	./test/chainsaw/teardown.sh

test-e2e-diagnose:
	KUBECONFIG=$(E2E_KUBECONFIG) ./test/chainsaw/diagnose.sh

# test-e2e-all forces teardown even on failure so no cluster leaks across runs.
# On failure, diagnose runs before teardown so cluster state is captured
# while the API server is still reachable.
test-e2e-all:
	$(MAKE) test-e2e-setup
	{ $(MAKE) test-e2e && $(MAKE) test-e2e-recovery ; } ; rc=$$? ; \
	if [ $$rc -ne 0 ]; then $(MAKE) test-e2e-diagnose || true ; fi ; \
	$(MAKE) test-e2e-teardown ; exit $$rc

# Diagnostic target: retains the cluster for inspection after test failures.
# The cluster's kubeconfig lives at $(E2E_KUBECONFIG) — export it first:
#   export KUBECONFIG=$(E2E_KUBECONFIG)
# then use kubectl normally.
test-e2e-keep:
	$(MAKE) test-e2e-setup
	$(MAKE) test-e2e || true
	@echo "Cluster retained for inspection."
	@echo "  export KUBECONFIG=$(E2E_KUBECONFIG)"
	@echo "Run 'make test-e2e-teardown' when done."
