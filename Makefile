# Makefile for CodeMint CLI

# Build metadata
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Go build settings
BINARY := codemint
PKG := codemint.kanthorlabs.com/cmd/codemint
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(DATE)"

# Default target
.PHONY: all
all: build

# Build the binary
.PHONY: build
build:
	go build $(LDFLAGS) -o build/$(BINARY) $(PKG)

# Build with race detector (for development/testing)
.PHONY: build-race
build-race:
	go build -race $(LDFLAGS) -o build/$(BINARY) $(PKG)

# Run tests
.PHONY: test
test:
	go test ./...

# Run tests with race detector
.PHONY: test-race
test-race:
	go test -race ./...

# Run tests with coverage
.PHONY: test-coverage
test-coverage:
	go test -coverprofile=build/coverage.out ./...
	go tool cover -html=build/coverage.out -o build/coverage.html

# Clean build artifacts
.PHONY: clean
clean:
	rm -f build/$(BINARY) build/coverage.out build/coverage.html

# Install the binary to $GOPATH/bin
.PHONY: install
install:
	go install $(LDFLAGS) $(PKG)

# Format code
.PHONY: fmt
fmt:
	go fmt ./...

# Lint code (requires golangci-lint)
.PHONY: lint
lint:
	golangci-lint run

# Run the application
.PHONY: run
run: build
	./build/$(BINARY)

# Print version info
.PHONY: version
version:
	@echo "Version: $(VERSION)"
	@echo "Commit:  $(COMMIT)"
	@echo "Date:    $(DATE)"

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build         - Build the binary"
	@echo "  build-race    - Build with race detector"
	@echo "  test          - Run tests"
	@echo "  test-race     - Run tests with race detector"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  clean         - Remove build artifacts"
	@echo "  install       - Install to GOPATH/bin"
	@echo "  fmt           - Format code"
	@echo "  lint          - Lint code (requires golangci-lint)"
	@echo "  run           - Build and run the application"
	@echo "  version       - Print build version info"
	@echo "  help          - Show this help message"
