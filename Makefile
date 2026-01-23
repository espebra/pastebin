.PHONY: build run test lint clean docker package-deb package-rpm packages

BINARY_NAME=pastebin
BUILD_DIR=bin

# Go commands
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
GOVET=$(GOCMD) vet

# Version from git tag (commit info is embedded automatically by Go)
VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || echo "")

# Build flags
LDFLAGS=-ldflags="-s -w -X main.Version=$(VERSION)"

build:
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/pastebin

run: build
	./$(BUILD_DIR)/$(BINARY_NAME)

test:
	$(GOTEST) -v -race -cover ./...

lint:
	golangci-lint run ./...

vet:
	$(GOVET) ./...

clean:
	rm -rf $(BUILD_DIR)

tidy:
	$(GOMOD) tidy

docker:
	docker build -t $(BINARY_NAME):latest .

# Cross-compilation targets
build-linux-amd64:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/pastebin

build-linux-arm64:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/pastebin

build-darwin-amd64:
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/pastebin

build-darwin-arm64:
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/pastebin

build-all: build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64

# Package building (requires nfpm: go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest)
DIST_DIR=dist
PKG_VERSION ?= $(shell echo $(VERSION) | sed 's/^v//' || echo "0.0.0")

package-deb-amd64: build-linux-amd64
	@mkdir -p $(DIST_DIR)
	@cp $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(DIST_DIR)/
	VERSION=$(PKG_VERSION) GOARCH=amd64 nfpm package -p deb -t $(DIST_DIR)/

package-deb-arm64: build-linux-arm64
	@mkdir -p $(DIST_DIR)
	@cp $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(DIST_DIR)/
	VERSION=$(PKG_VERSION) GOARCH=arm64 nfpm package -p deb -t $(DIST_DIR)/

package-rpm-amd64: build-linux-amd64
	@mkdir -p $(DIST_DIR)
	@cp $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(DIST_DIR)/
	VERSION=$(PKG_VERSION) GOARCH=amd64 nfpm package -p rpm -t $(DIST_DIR)/

package-rpm-arm64: build-linux-arm64
	@mkdir -p $(DIST_DIR)
	@cp $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(DIST_DIR)/
	VERSION=$(PKG_VERSION) GOARCH=arm64 nfpm package -p rpm -t $(DIST_DIR)/

packages: package-deb-amd64 package-deb-arm64 package-rpm-amd64 package-rpm-arm64
