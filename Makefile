.PHONY: build run test lint clean docker

BINARY_NAME=pastebin
BUILD_DIR=bin

# Go commands
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
GOVET=$(GOCMD) vet

# Build flags
LDFLAGS=-ldflags="-s -w"

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
