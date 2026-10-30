# HELP
# thanks to https://marmelab.com/blog/2016/02/29/auto-documented-makefile.html

.DEFAULT_GOAL := help

.PHONY: help all build build-all test test-multicast coverage cover cross lint lint-ci fmt fmt-check vet vuln docs check install clean e2e

help: ## This help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

export GOWORK := off

APP_NAME    := knxctl
BIN_DIR     := bin
RELEASE_DIR := release
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
TAG         ?= $(shell git describe --tags --exact-match 2>/dev/null)
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null)
BUILD_DATE  ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     := -ldflags "-s -w -X 'main.version=$(VERSION)' -X 'main.tag=$(TAG)' -X 'main.commit=$(COMMIT)' -X 'main.buildDate=$(BUILD_DATE)'"
COVERAGE_MIN := 85

GO_MOD_VERSION := $(shell sed -nE 's/^go ([0-9]+\.[0-9]+).*/\1/p' go.mod | head -n1)
GOFMT_CI = $(shell GOTOOLCHAIN=go$(GO_MOD_VERSION).0 go env GOROOT)/bin/gofmt

# Release targets: linux amd64/arm64/armv7, darwin amd64/arm64, windows amd64/arm64.
PLATFORMS := linux/amd64 linux/arm64 linux/arm darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

all: check build ## Check and build

build: ## Build ./bin/knxctl
	@mkdir -p $(BIN_DIR)
	@go build $(LDFLAGS) -o $(BIN_DIR)/$(APP_NAME) .

build-all: ## Build for every release platform into ./release
	@mkdir -p $(RELEASE_DIR)
	@for p in $(PLATFORMS); do os=$${p%/*}; cpu=$${p#*/}; ext=; [ $$os = windows ] && ext=.exe; \
		name=$(APP_NAME)-$(VERSION)-$$os-$$cpu; [ $$cpu = arm ] && name=$(APP_NAME)-$(VERSION)-$$os-armv7; \
		echo "build $$name$$ext"; GOOS=$$os GOARCH=$$cpu GOARM=7 go build $(LDFLAGS) -o $(RELEASE_DIR)/$$name$$ext . || exit 1; done

test: ## Run the tests with the race detector (routing tests skip without multicast loopback)
	@go test -count=1 -race ./...

test-multicast: ## Run the tests, failing when multicast loopback does not work
	@KNX_REQUIRE_MULTICAST=1 go test -count=1 -race ./...

coverage: ## Whole-module coverage (coverage.out); fails below COVERAGE_MIN
	@go test -count=1 -race -covermode=atomic -coverpkg=./... -coverprofile=coverage.out ./...
	@total=$$(go tool cover -func=coverage.out | tail -1 | awk '{print $$3}' | tr -d %); \
	echo "total $$total% (minimum $(COVERAGE_MIN)%)"; awk -v t=$$total -v m=$(COVERAGE_MIN) 'BEGIN { exit (t+0 < m+0) }'

cover: coverage ## Open the coverage report
	@go tool cover -html=coverage.out

cross: ## Typecheck for every release platform (32-bit ARM included)
	@for p in $(PLATFORMS); do GOOS=$${p%/*} GOARCH=$${p#*/} go vet ./... || exit 1; done

lint: ## staticcheck (this platform and Linux)
	@staticcheck ./...
	@GOOS=linux staticcheck ./...

lint-ci: ## golangci-lint
	@golangci-lint run ./...

fmt: ## gofmt with the go.mod Go version
	@$(GOFMT_CI) -w .

fmt-check: ## Fail when gofmt would change a file
	@out="$$($(GOFMT_CI) -l .)$$(gofmt -l .)"; [ -z "$$out" ] || { echo "Not formatted: $$out"; exit 1; }

vet: ## go vet
	@go vet ./...
	@GOOS=linux go vet ./...
	@GOOS=windows go vet ./...

vuln: ## govulncheck
	@govulncheck ./...

docs: ## Regenerate docs/cli.md from the --help of every command
	@scripts/gen-cli-doc.sh

check: fmt-check lint lint-ci vet cross vuln test coverage ## Everything CI checks

e2e: ## Build the binary and run the e2e suite against it
	@mkdir -p bin
	@go build $(LDFLAGS) -o bin/knxctl .
	@KNXCTL_E2E_BIN=$(CURDIR)/bin/knxctl KNXCTL_E2E_VERSION=$(VERSION) go test -count=1 -tags=e2e ./e2e/...

install: build ## Install into /usr/local/bin
	@install -m 0755 $(BIN_DIR)/$(APP_NAME) /usr/local/bin/$(APP_NAME)

clean: ## Remove build output
	@rm -rf $(BIN_DIR) $(RELEASE_DIR) coverage.out
