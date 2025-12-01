# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## About Forge

Forge is a batteries-included Go framework for building production-ready microservices, inspired by DropWizard but designed for Go's strengths. It provides opinionated defaults while maintaining flexibility through a pluggable architecture.

## Common Commands

### Building and Testing
```bash
# Build the framework
task build
# OR
go build ./...

# Run all unit tests
task test
# OR
go test ./...

# Run tests with verbose output
task test:verbose

# Run tests for specific package
task test:package PKG=./framework
task test:package PKG=./bundles/postgresql

# Check for race conditions
task test:race

# Run with coverage report
task test:coverage
# Opens coverage.html in browser

# Run integration tests (requires Docker)
task test:integration
# Starts PostgreSQL + Redis, runs tests, cleans up

# Watch mode (requires 'entr')
task test:watch

# See all available tasks
task --list
```

### Code Quality
```bash
# Format code
task fmt
# OR
go fmt ./...

# Vet code for issues
task vet

# Run all linters
task lint

# Auto-fix lint issues
task lint:fix
```

### Module Management
```bash
# Tidy dependencies
task deps:tidy
# OR
go mod tidy

# Download dependencies
task deps

# Verify dependencies
task deps:verify

# Update all dependencies
task deps:update
```

## Architecture Overview

Forge follows Clean Architecture principles with these key components:

### Core Framework (`framework/`)
- **App (`framework/app.go`)**: Main application struct with lifecycle management, component registration, and graceful shutdown orchestration
- **Component Interface**: Services implement `Start(ctx)` and `Stop(ctx)` methods for lifecycle management
- **Bundle Interface**: Reusable functionality packages that initialize themselves with the app
- **Logging Manager**: Structured logging with OpenTelemetry integration
- **Observability Manager**: Built-in tracing, metrics, and telemetry
- **Health Registry**: Comprehensive health checks with liveness/readiness endpoints

### Configuration (`config/`)
- **BaseConfig**: Common configuration for all services using environment variables and YAML
- Environment-based config with sensible defaults (development, staging, production)
- Built-in validation with the `Validator` interface
- Configuration includes gRPC/HTTP server settings, timeouts, and observability settings

### Key Patterns
- **Graceful Lifecycle**: App orchestrates startup/shutdown with proper ordering and timeout handling
- **Component Composition**: Business logic implements framework interfaces for clean integration
- **Bundle System**: Pre-built integrations (PostgreSQL, Redis, etc.) can be added via `WithBundle()`
- **Health Checks**: Components can contribute health checks via `HealthContributor` interface
- **Hook System**: Startup and shutdown hooks for custom initialization/cleanup

### Application Structure
```
framework/          # Core framework implementation
├── app.go         # Main App struct and lifecycle management
├── logging.go     # Structured logging with OpenTelemetry
├── observability.go  # Tracing and metrics setup
└── shutdown.go    # Graceful shutdown orchestration

config/            # Configuration management
└── base.go        # BaseConfig with environment integration

bundles/           # Pre-built integrations (empty in current codebase)
adapters/          # Infrastructure adapters
clients/           # External service clients
health/            # Health check system
errors/            # Error handling utilities
examples/          # Example implementations
```

### Creating a Service
Services typically:
1. Define a config struct embedding `config.BaseConfig`
2. Implement business components with `Component` interface
3. Use `framework.New()` with options to build the app:
   - `WithConfig()`: Service configuration
   - `WithComponent()`: Business logic components
   - `WithBundle()`: Pre-built integrations
   - `WithGRPCRegistrar()`: gRPC service registration
   - `WithHealthContributor()`: Health checks

### Server Configuration
- **gRPC Server**: Runs on `GRPC_ADDR` (default `:8080`)
- **HTTP Server**: Health endpoints on `HTTP_ADDR` (default `:8081`)
  - `/health`: Overall health status
  - `/health/ready`: Readiness probe
  - `/health/live`: Liveness probe

### Observability
Built-in support for:
- **OpenTelemetry**: Distributed tracing with configurable sampling
- **Structured Logging**: JSON logging with zerolog
- **Metrics**: Ready for Prometheus integration
- **Health Checks**: Kubernetes-compatible health endpoints
- We want to be "batteries included" but also lightweight, so we need to evaluate 3rd party modules before adding them to the project.