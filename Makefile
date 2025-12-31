.PHONY: build run test test-coverage test-race lint fmt vet clean mocks backtest help

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GORUN=$(GOCMD) run
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOVET=$(GOCMD) vet

# Binary name
BINARY_NAME=quant-bot
BINARY_PATH=./bin/$(BINARY_NAME)

# Main package
MAIN_PKG=./cmd/bot

# Version info
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME = $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT = $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Build flags
LDFLAGS=-ldflags "-s -w -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)"

# Default target
.DEFAULT_GOAL := help

## help: Show this help message
help:
	@echo "Quant Trading Bot - MES/MGC"
	@echo ""
	@echo "Usage:"
	@echo "  make <target>"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'

## build: Build the binary
build:
	@echo "Building..."
	@mkdir -p bin
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_PATH) $(MAIN_PKG)
	@echo "Built: $(BINARY_PATH)"

## run: Run the bot (dev mode)
run:
	$(GORUN) $(MAIN_PKG)

## test: Run all tests
test:
	$(GOTEST) -v ./...

## test-coverage: Run tests with coverage report
test-coverage:
	@mkdir -p coverage
	$(GOTEST) -v -coverprofile=coverage/coverage.out ./...
	$(GOCMD) tool cover -html=coverage/coverage.out -o coverage/coverage.html
	@echo "Coverage report: coverage/coverage.html"

## test-race: Run tests with race detector
test-race:
	$(GOTEST) -v -race ./...

## test-short: Run only short tests
test-short:
	$(GOTEST) -v -short ./...

## test-pkg: Run tests for a specific package (usage: make test-pkg PKG=./internal/risk)
test-pkg:
	$(GOTEST) -v $(PKG)

## test-run: Run a specific test (usage: make test-run TEST=TestPositionSizer)
test-run:
	$(GOTEST) -v -run $(TEST) ./...

## lint: Run golangci-lint
lint:
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci-lint/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run ./...

## lint-fix: Run golangci-lint with auto-fix
lint-fix:
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci-lint/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run --fix ./...

## fmt: Format code
fmt:
	$(GOFMT) -s -w .

## vet: Run go vet
vet:
	$(GOVET) ./...

## mod-tidy: Tidy go modules
mod-tidy:
	$(GOMOD) tidy

## mod-download: Download go modules
mod-download:
	$(GOMOD) download

## mocks: Generate mock files (requires mockgen)
mocks:
	@which mockgen > /dev/null || (echo "Installing mockgen..." && go install go.uber.org/mock/mockgen@latest)
	@echo "Generating mocks..."
	@# Add mockgen commands here as interfaces are defined
	@# mockgen -source=internal/risk/engine.go -destination=internal/risk/mocks/engine_mock.go -package=mocks
	@echo "Done generating mocks"

## backtest: Run backtest (usage: make backtest DATA=./data/mes_2024.csv)
backtest:
	@if [ -z "$(DATA)" ]; then \
		echo "Usage: make backtest DATA=./data/mes_2024.csv"; \
		exit 1; \
	fi
	$(GORUN) $(MAIN_PKG) backtest --data=$(DATA)

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf bin/
	@rm -rf coverage/
	@rm -f coverage.out
	@echo "Done"

## deps: Install development dependencies
deps:
	@echo "Installing development dependencies..."
	go install github.com/golangci-lint/golangci-lint/cmd/golangci-lint@latest
	go install go.uber.org/mock/mockgen@latest
	@echo "Done"

## check: Run all checks (fmt, vet, lint, test)
check: fmt vet lint test
	@echo "All checks passed!"

## ci: Run CI pipeline locally
ci: mod-tidy fmt vet lint test-race
	@echo "CI checks passed!"

## docker-build: Build Docker image
docker-build:
	docker build -t $(BINARY_NAME):latest .

## docker-run: Run Docker container
docker-run:
	docker run --rm -it $(BINARY_NAME):latest
