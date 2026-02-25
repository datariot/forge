# Forge

A batteries-included Go framework for building production-ready microservices.

## Overview

Forge is inspired by DropWizard but designed specifically for Go's strengths - interfaces, goroutines, and clean composition. It provides opinionated defaults while maintaining flexibility through a pluggable architecture.

## Features

- **Clean Architecture**: Component-based design with clear separation of concerns
- **Observability Built-in**: OpenTelemetry tracing, structured logging, Prometheus metrics
- **Health Checks**: Comprehensive liveness and readiness checks
- **Graceful Lifecycle**: Sophisticated startup and shutdown orchestration
- **Configuration Management**: Environment-based config with validation
- **Database Integration**: Transaction-safe PostgreSQL patterns
- **Event Publishing**: Redis Streams integration
- **Security First**: No hardcoded credentials, explicit validation requirements

## Quick Start

```go
package main

import (
    "context"
    "github.com/datariot/forge/framework"
    "github.com/datariot/forge/bundles/postgresql"
)

func main() {
    cfg := MustLoadConfig()

    myComponent := NewMyComponent(cfg)

    app := framework.New(
        framework.WithConfig(&cfg.BaseConfig),
        framework.WithVersion("1.0.0"),
        framework.WithComponent(myComponent),
        framework.WithBundle(postgresql.Bundle()),
    )

    if err := app.Run(context.Background()); err != nil {
        log.Fatal(err)
    }
}
```

## Installation

```bash
go get github.com/datariot/forge
```

## Development

### Prerequisites

- Go 1.25+
- Docker & Docker Compose (for integration tests)
- [Task](https://taskfile.dev) (optional, for task runner)

### Common Commands

```bash
# Run tests
task test
# OR
go test ./...

# Run tests with coverage
task test:coverage

# Run integration tests (requires Docker)
task test:integration

# Build framework and examples
task build:all

# Format and lint code
task lint

# See all available tasks
task --list
```

### Testing

**Unit Tests** (no external dependencies):
```bash
task test
```

**Integration Tests** (requires Docker):
```bash
task docker:up           # Start PostgreSQL + Redis
task test:integration    # Run integration tests
task docker:down         # Stop services
```

**Coverage Report**:
```bash
task test:coverage       # Generates coverage.html
```

Current test coverage: **70.0%** (Target: 70%+)

See [TESTING.md](TESTING.md) for comprehensive testing strategy.

## Architecture

Forge follows Clean Architecture principles:

- **Framework**: Core application lifecycle and interfaces
- **Bundles**: Pre-built integrations (PostgreSQL, Redis, Prometheus, etc.)
- **Components**: Your business logic implementing framework interfaces
- **Adapters**: Infrastructure integrations
- **Config**: Environment-driven configuration management

## Documentation

- [Getting Started](docs/getting-started.md)
- [Component Development](docs/components.md)
- [Configuration Guide](docs/configuration.md)
- [Health Checks](docs/health-checks.md)
- [Examples](examples/)
- [Development Guide](CLAUDE.md)

## CI/CD

GitHub Actions automatically runs on all PRs:
- Unit tests with race detection
- Code formatting checks
- Linting (go vet + golangci-lint)
- Build verification (framework + examples)
- Coverage reporting (70% threshold)

See [.github/workflows/test.yml](.github/workflows/test.yml) for details.

## License

MIT License - see LICENSE file for details.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Add tests for your changes
4. Ensure `task ci` passes
5. Submit a pull request

All contributions must:
- Include tests (maintain 70%+ coverage)
- Follow Go best practices
- Include documentation
- Pass CI/CD checks
