VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

LDFLAGS = -s -w \
	-X main.Version=$(VERSION) \
	-X main.BuildTime=$(BUILD_TIME) \
	-X main.GitCommit=$(GIT_COMMIT)

BINARY = abc-node-probe

.PHONY: build \
	build-linux-amd64 build-linux-arm64 \
	build-darwin-amd64 build-darwin-arm64 \
	build-windows-amd64 build-windows-arm64 \
	build-all \
	test test-unit lint release clean help

.DEFAULT_GOAL := help

help: ## Show this help message
	@echo "abc-node-probe build targets"
	@echo ""
	@echo "Usage: make <target>"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z0-9_-]+:.*##/ { printf "  %-22s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# Build for host platform
build: ## Build for the host OS and architecture
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o $(BINARY) .

# Linux
build-linux-amd64: ## Cross-compile for Linux amd64 -> dist/
	@mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o dist/$(BINARY)-linux-amd64 .

build-linux-arm64: ## Cross-compile for Linux arm64 -> dist/
	@mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o dist/$(BINARY)-linux-arm64 .

# macOS
build-darwin-amd64: ## Cross-compile for macOS amd64 -> dist/
	@mkdir -p dist
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o dist/$(BINARY)-darwin-amd64 .

build-darwin-arm64: ## Cross-compile for macOS arm64 (Apple Silicon) -> dist/
	@mkdir -p dist
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o dist/$(BINARY)-darwin-arm64 .

# Windows
build-windows-amd64: ## Cross-compile for Windows amd64 -> dist/
	@mkdir -p dist
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o dist/$(BINARY)-windows-amd64.exe .

build-windows-arm64: ## Cross-compile for Windows arm64 -> dist/
	@mkdir -p dist
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o dist/$(BINARY)-windows-arm64.exe .

# All platforms
build-all: build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 build-windows-amd64 build-windows-arm64 ## Build all platform/arch combinations -> dist/

test: ## Run all tests
	CGO_ENABLED=0 go test ./...

test-unit: ## Run unit tests only (no network, no /proc required)
	CGO_ENABLED=0 go test -short ./...

test-integration: ## Run integration tests (cross-platform; SMART subtests require Linux + root)
	CGO_ENABLED=0 go test -tags integration ./...

lint: ## Run golangci-lint
	golangci-lint run

release: build-all ## Build all platforms and generate sha256sums.txt in dist/
	cd dist && sha256sum \
		$(BINARY)-linux-amd64 \
		$(BINARY)-linux-arm64 \
		$(BINARY)-darwin-amd64 \
		$(BINARY)-darwin-arm64 \
		$(BINARY)-windows-amd64.exe \
		$(BINARY)-windows-arm64.exe \
		> sha256sums.txt
	@echo "Release artifacts in dist/"

# Backward-compat aliases
build-amd64: build-linux-amd64
build-arm64: build-linux-arm64

clean: ## Remove build artifacts
	rm -f $(BINARY)
	rm -rf dist/
