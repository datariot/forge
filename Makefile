.PHONY: help test test-unit test-integration test-coverage clean docker-up docker-down docker-clean

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Testing targets

test: test-unit ## Run all unit tests (no external dependencies)

test-unit: ## Run unit tests only
	@echo "Running unit tests..."
	go test -v -race ./...

test-integration: docker-up ## Run integration tests (requires Docker)
	@echo "Waiting for services to be healthy..."
	@sleep 5
	@echo "Running integration tests..."
	DATABASE_URL="postgres://forge:forge-test-password@localhost:5432/forge_test?sslmode=disable" \
	REDIS_URL="redis://localhost:6379/0" \
	go test -v -tags=integration ./bundles/...
	@$(MAKE) docker-down

test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"
	@go tool cover -func=coverage.out | grep total

test-watch: ## Run tests on file changes (requires entr)
	@echo "Watching for changes..."
	@find . -name '*.go' | entr -c make test-unit

# Docker targets

docker-up: ## Start Docker services for testing
	@echo "Starting test infrastructure..."
	docker-compose -f docker-compose.test.yml up -d
	@echo "Waiting for services to be ready..."
	@sleep 3

docker-down: ## Stop Docker services
	@echo "Stopping test infrastructure..."
	docker-compose -f docker-compose.test.yml down

docker-clean: ## Stop and remove Docker volumes
	@echo "Cleaning test infrastructure..."
	docker-compose -f docker-compose.test.yml down -v

docker-logs: ## Show Docker service logs
	docker-compose -f docker-compose.test.yml logs -f

# Build targets

build: ## Build the framework
	@echo "Building framework..."
	go build ./...

build-examples: ## Build all example services
	@echo "Building examples..."
	@for dir in examples/*/; do \
		echo "Building $$dir..."; \
		(cd "$$dir" && go build .) || exit 1; \
	done
	@echo "✅ All examples built successfully"

# Code quality targets

fmt: ## Format code
	@echo "Formatting code..."
	go fmt ./...

vet: ## Run go vet
	@echo "Running go vet..."
	go vet ./...

lint: fmt vet ## Run all linters
	@echo "Running golangci-lint..."
	@golangci-lint run --timeout=5m || echo "golangci-lint not installed"

# Cleanup targets

clean: ## Clean build artifacts and coverage files
	@echo "Cleaning..."
	rm -f coverage.out coverage.html
	find examples -type f -executable -delete
	find . -name "*.test" -delete

clean-all: clean docker-clean ## Clean everything including Docker volumes
	@echo "✅ Cleaned all artifacts and Docker volumes"

# Development targets

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	go mod download

tidy: ## Tidy dependencies
	@echo "Tidying dependencies..."
	go mod tidy

verify: ## Verify dependencies
	@echo "Verifying dependencies..."
	go mod verify

# CI targets

ci: fmt vet test-unit build build-examples ## Run all CI checks (no Docker required)
	@echo "✅ All CI checks passed"

ci-full: ci test-integration ## Run full CI including integration tests
	@echo "✅ Full CI passed"
