.PHONY: build check-skills check-skill-contracts check-privacy clean install test lint fmt help update-meta

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt

# Binary name
BINARY_NAME=feishu-cli

# Build directory
BUILD_DIR=bin

# Version info
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

## build: Build the binary
build:
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .

## check-skills: Build current source; check Skill YAML, examples, ownership and offline behavior
check-skills: build
	python3 scripts/check_skills.py ./$(BUILD_DIR)/$(BINARY_NAME)
	python3 scripts/run_skill_regressions.py ./$(BUILD_DIR)/$(BINARY_NAME)

## check-skill-contracts: Build current source and run offline CLI contracts (no live Feishu API)
check-skill-contracts: build
	python3 scripts/check_skill_contracts.py ./$(BUILD_DIR)/$(BINARY_NAME)

## check-privacy: Scan tracked files for internal emails, tenant-prefixed Feishu domains and real-looking tokens
check-privacy:
	python3 scripts/check_privacy.py

## install: Install the binary to $GOPATH/bin
install:
	$(GOBUILD) $(LDFLAGS) -o $(GOPATH)/bin/$(BINARY_NAME) .

## clean: Clean build artifacts
clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)

## test: Run tests
test:
	$(GOTEST) -v ./...

## coverage: Run tests with coverage
coverage:
	$(GOTEST) -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

## lint: Run linter
lint:
	golangci-lint run ./...

## fmt: Format code
fmt:
	$(GOFMT) -w -s .

## tidy: Tidy go modules
tidy:
	$(GOMOD) tidy

## deps: Download dependencies
deps:
	$(GOMOD) download

## build-all: Build for multiple platforms
build-all: build-linux build-darwin build-windows

build-linux:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 .

build-darwin:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 .
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 .

build-windows:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe .

## run: Run the application
run: build
	./$(BUILD_DIR)/$(BINARY_NAME)

## update-meta: Fetch latest API metadata from open.feishu.cn (build-time embed baseline)
update-meta:
	@echo "Fetching API metadata from open.feishu.cn..."
	@curl -sS --max-time 10 "https://open.feishu.cn/api/tools/open/api_definition?protocol=meta" | \
		python3 -c "import sys,json; json.dump(json.load(sys.stdin)['data'], open('internal/registry/meta_data.json','w'), ensure_ascii=False)" \
		&& echo "Done. $$(wc -c < internal/registry/meta_data.json | tr -d ' ') bytes written."
	@echo "Runtime overlay still uses the same public endpoint (opt-out: FEISHU_CLI_REMOTE_META=off)."

## init-config: Initialize configuration file
init-config: build
	./$(BUILD_DIR)/$(BINARY_NAME) init-config
