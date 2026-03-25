.PHONY: build build-dxt pack-dxt test clean run-dev release-snapshot run-docker run docker-compose-up docker-compose-down lint docker-test cli install-cli

# Variables
BINARY_NAME=mcp-trino
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_DIR=bin

# Build the application (single binary for local development)
build:
	mkdir -p $(BUILD_DIR)
	go build -ldflags "-X main.Version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd

# Build CLI binary (alias for build)
cli: build

# Install CLI binary to $GOPATH/bin or $HOME/go/bin
install-cli: build
	@echo "Installing $(BINARY_NAME) to $$(go env GOPATH)/bin/..."
	@mkdir -p $$(go env GOPATH)/bin
	@cp $(BUILD_DIR)/$(BINARY_NAME) $$(go env GOPATH)/bin/
	@echo "Installed successfully. You can now run: $(BINARY_NAME) --version"

# Build all platform-specific binaries for DXT packaging
build-dxt:
	mkdir -p server
	@echo "Building platform-specific binaries for DXT..."
	GOOS=darwin GOARCH=arm64 go build -ldflags "-X main.Version=$(VERSION)" -o server/$(BINARY_NAME)-darwin-arm64 ./cmd
	GOOS=darwin GOARCH=amd64 go build -ldflags "-X main.Version=$(VERSION)" -o server/$(BINARY_NAME)-darwin-amd64 ./cmd
	GOOS=linux GOARCH=amd64 go build -ldflags "-X main.Version=$(VERSION)" -o server/$(BINARY_NAME)-linux-amd64 ./cmd
	GOOS=windows GOARCH=amd64 go build -ldflags "-X main.Version=$(VERSION)" -o server/$(BINARY_NAME)-windows-amd64.exe ./cmd
	chmod +x server/$(BINARY_NAME)-*
	@echo "All platform binaries built in server/ directory"

# Package DXT extension
pack-dxt: build-dxt
	@echo "Packaging DXT extension..."
	dxt pack
	@echo "DXT package created: $(BINARY_NAME).dxt"

# Run tests
test:
	go test ./...

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)
	rm -rf server
	rm -f $(BINARY_NAME).dxt $(BINARY_NAME)-*.dxt

# Run the application in development mode
run-dev:
	go run ./cmd

# Create a release snapshot using GoReleaser
release-snapshot:
	goreleaser release --snapshot --clean

# Run the application using the built binary
run:
	./$(BUILD_DIR)/$(BINARY_NAME)

# Build and run Docker image
run-docker: build
	docker build -t $(BINARY_NAME):$(VERSION) .
	docker run -p 8080:8080 $(BINARY_NAME):$(VERSION)

# Start the application with Docker Compose
docker-compose-up:
	docker-compose up -d

# Stop Docker Compose services
docker-compose-down:
	docker-compose down

# Run linting checks (same as CI)
lint:
	@echo "Running linters..."
	@go mod tidy
	@if ! git diff --quiet go.mod go.sum; then echo "go.mod or go.sum is not tidy, run 'go mod tidy'"; git diff go.mod go.sum; exit 1; fi
	@if ! command -v golangci-lint &> /dev/null; then echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; fi
	@golangci-lint run --timeout=5m

# Run tests in Docker
docker-test:
	docker build -f Dockerfile.test -t $(BINARY_NAME)-test:$(VERSION) .
	docker run --rm $(BINARY_NAME)-test:$(VERSION)

# Default target
all: clean build
