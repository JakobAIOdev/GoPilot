.PHONY: build test install clean lint

BINARY := gopilot
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.version=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/gopilot

test:
	go test ./... -v

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/gopilot

clean:
	rm -f $(BINARY) main
	go clean -cache -testcache

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not found, install: https://golangci-lint.run/usage/install/"; exit 1; }
	golangci-lint run ./...
