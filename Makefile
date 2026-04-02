# SuperTerminal Makefile

# Version (read from VERSION file)
VERSION := $(shell cat VERSION 2>/dev/null || echo "v0.4.0")

# Binary name
BINARY := superterminal

# Build directory
BUILD_DIR := ./build

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GORUN := $(GOCMD) run
GOCLEAN := $(GOCMD) clean
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod

# Build flags
LDFLAGS := -ldflags "-s -w -X main.Version=$(VERSION)"

# Platform-specific builds
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

# Default target
.PHONY: all
all: clean deps build

# Install dependencies
.PHONY: deps
deps:
	$(GOMOD) tidy

# Build for current platform
.PHONY: build
build:
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/superterminal

# Build with debug info
.PHONY: build-debug
build-debug:
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY) ./cmd/superterminal

# Build for all platforms
.PHONY: build-all
build-all: clean deps
	for platform in $(PLATFORMS); do \
		IFS='/' read -r GOOS GOARCH <<< "$$platform"; \
		output=$(BUILD_DIR)/$(BINARY)-$$GOOS-$$GOARCH; \
		if [ "$$GOOS" = "windows" ]; then \
			output=$$output.exe; \
		fi; \
		CGO_ENABLED=0 GOOS=$$GOOS GOARCH=$$GOARCH $(GOBUILD) $(LDFLAGS) -o $$output ./cmd/superterminal; \
		echo "Built $$output"; \
	done

# Run locally (for development)
.PHONY: run
run:
	$(GORUN) ./cmd/superterminal

# Run with Web UI
.PHONY: run-web
run-web:
	$(GORUN) ./cmd/superterminal --web --port 8080

# Run Web UI only
.PHONY: run-web-only
run-web-only:
	$(GORUN) ./cmd/superterminal --web-only --port 8080

# Run tests
.PHONY: test
test:
	$(GOTEST) -v ./...

# Clean build artifacts
.PHONY: clean
clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)

# Install binary to system
.PHONY: install
install: build
	cp $(BUILD_DIR)/$(BINARY) /usr/local/bin/

# Format code
.PHONY: fmt
fmt:
	$(GOCMD) fmt ./...

# Lint code
.PHONY: lint
lint:
	golangci-lint run ./...

# Generate documentation
.PHONY: docs
docs:
	go doc -all ./internal/engine > docs/engine.md
	go doc -all ./internal/tui > docs/tui.md
	go doc -all ./internal/webui > docs/webui.md

# Create distribution package
.PHONY: dist
dist: build-all
	cd $(BUILD_DIR) && \
	for f in $(BINARY)-*; do \
		tar -czvf $$f.tar.gz $$f; \
	done

# Development watch mode (requires entr or similar)
.PHONY: watch
watch:
	find . -name '*.go' | entr -r $(MAKE) run

# Create release (uses scripts/release.sh)
.PHONY: release
release:
	@if [ -z "$(NEW_VERSION)" ]; then \
		echo "Usage: make release NEW_VERSION=vX.Y.Z"; \
		exit 1; \
	fi
	./scripts/release.sh $(NEW_VERSION)

# Development mode (build + run with debug)
.PHONY: dev
dev: build-debug
	./build/$(BINARY) --debug

# Test with coverage
.PHONY: test-coverage
test-coverage:
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Check version
.PHONY: version
version:
	@echo "SuperTerminal $(VERSION)"

# Help
.PHONY: help
help:
	@echo "SuperTerminal Makefile Targets:"
	@echo ""
	@echo "  all              Clean, install deps, and build"
	@echo "  deps             Install dependencies (go mod tidy)"
	@echo "  build            Build for current platform"
	@echo "  build-debug      Build with debug symbols"
	@echo "  build-all        Build for all platforms"
	@echo "  run              Run locally (development)"
	@echo "  run-web          Run with Web UI enabled"
	@echo "  run-web-only     Run Web UI only mode"
	@echo "  dev              Development mode (debug build + run)"
	@echo "  test             Run tests"
	@echo "  test-coverage    Run tests with coverage report"
	@echo "  clean            Remove build artifacts"
	@echo "  install          Install to /usr/local/bin"
	@echo "  fmt              Format Go code"
	@echo "  lint             Run golangci-lint"
	@echo "  docs             Generate documentation"
	@echo "  dist             Create distribution packages"
	@echo "  release          Create release (NEW_VERSION=vX.Y.Z)"
	@echo "  version          Show current version"
	@echo "  help             Show this help"