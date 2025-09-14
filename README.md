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

## License

MIT License - see LICENSE file for details.