---
layout: home
title: "Forge - Production-Ready Go Microservices"
---

# Forge Framework

A batteries-included Go framework for building production-ready microservices, inspired by DropWizard but designed specifically for Go's strengths.

## Key Features

- **🏗️ Clean Architecture** - Component-based design with clear separation of concerns
- **🔍 Observability Built-in** - OpenTelemetry tracing, structured logging, Prometheus metrics
- **💚 Health Checks** - Comprehensive liveness and readiness checks
- **🔄 Graceful Lifecycle** - Sophisticated startup and shutdown orchestration
- **⚙️ Configuration Management** - Automatic loading from files and environment
- **🗄️ Database Integration** - Production-ready PostgreSQL and Redis bundles
- **🔐 Security First** - JWT authentication, TLS enforcement, secure defaults
- **📊 Enterprise Observability** - Prometheus metrics with Grafana dashboards

## Quick Start

```go
package main

import (
    "context"
    "log"

    "github.com/datariot/forge/config"
    "github.com/datariot/forge/framework"
    "github.com/datariot/forge/health"
)

type MyService struct{}

func (s *MyService) Start(ctx context.Context) error {
    log.Printf("MyService started")
    return nil
}

func (s *MyService) Stop(ctx context.Context) error {
    log.Printf("MyService stopping")
    return nil
}

func (s *MyService) HealthChecks() []health.Check {
    return []health.Check{
        health.NewAlwaysHealthyCheck("my-service"),
    }
}

func main() {
    cfg := config.DefaultBaseConfig()
    cfg.ServiceName = "my-service"

    app, err := framework.New(
        framework.WithConfig(&cfg),
        framework.WithVersion("1.0.0"),
        framework.WithComponent(&MyService{}),
        framework.WithHealthContributor(&MyService{}),
    )
    if err != nil {
        log.Fatal(err)
    }

    if err := app.Run(context.Background()); err != nil {
        log.Fatal(err)
    }
}
```

## Installation

```bash
go get github.com/datariot/forge
```

## Core Bundles

| Bundle | Purpose | Status |
|--------|---------|---------|
| **PostgreSQL** | Database connectivity with connection pooling | ✅ Production Ready |
| **Redis** | Caching, messaging, and distributed locking | ✅ Production Ready |
| **JWT Auth** | Secure service-to-service authentication | ✅ Production Ready |
| **HTTP Client** | Resilient HTTP communication with circuit breakers | ✅ Production Ready |
| **Prometheus** | Comprehensive metrics and observability | ✅ Production Ready |
| **Config Loader** | Automatic configuration management | ✅ Production Ready |

*All bundles include comprehensive security hardening, production-ready defaults, and extensive documentation.*

## Architecture

Forge follows Clean Architecture principles with these key concepts:

- **Components**: Your business logic implementing the `Component` interface
- **Bundles**: Pre-built integrations (PostgreSQL, Redis, JWT, etc.)
- **App**: Main application orchestrating lifecycle and dependencies
- **Health**: Comprehensive health check system with liveness/readiness
- **Observability**: Built-in OpenTelemetry tracing and Prometheus metrics

## Why Forge?

### 🎯 **Production-Ready Out of the Box**
- Security hardening applied throughout
- Comprehensive health checks and observability
- Graceful shutdown and error handling
- Enterprise-grade reliability patterns

### ⚡ **Lightweight Yet Complete**
- Only 16 direct dependencies (all essential)
- 94 total dependencies (62% reduction from initial)
- Zero cloud provider bloat or unnecessary complexity
- Fast build times and small binaries

### 🛡️ **Security First**
- JWT authentication with validation
- TLS enforcement and secure defaults
- Sensitive data redaction
- Comprehensive input validation

### 📈 **Enterprise Observability**
- Automatic HTTP/gRPC metrics collection
- Health check monitoring and alerting
- Distributed tracing with OpenTelemetry
- Pre-built Grafana dashboards

## Getting Started

1. **[Installation & Setup](getting-started.html)** - Get up and running in 5 minutes
2. **[Bundle Guide](bundles.html)** - Integrate database, caching, and authentication
3. **[Examples](examples.html)** - Working examples for common patterns
4. **[API Reference](api-reference.html)** - Complete API documentation

## Community

- **GitHub**: [github.com/datariot/forge](https://github.com/datariot/forge)
- **Issues**: [Report bugs and feature requests](https://github.com/datariot/forge/issues)
- **Discussions**: [Community discussions](https://github.com/datariot/forge/discussions)

## License

MIT License - see [LICENSE](https://github.com/datariot/forge/blob/main/LICENSE) for details.