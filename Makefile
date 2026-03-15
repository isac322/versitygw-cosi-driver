.PHONY: build test integration-test docker-build clean lint lint-fix

BINARY  := versitygw-cosi-driver
IMAGE   := versitygw-cosi-driver:latest

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
