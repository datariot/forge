---
layout: page
title: Getting Started
permalink: /getting-started/
---

# Getting Started with Forge

Get up and running with the Forge framework in just a few minutes.

## Installation

```bash
go mod init my-service
go get github.com/datariot/forge
```

## Your First Service

Create a simple service with health checks and graceful shutdown:

```go
package main

import (
    "context"
    "log"

    "github.com/datariot/forge/config"
    "github.com/datariot/forge/framework"
    "github.com/datariot/forge/health"
)

// UserService implements the Component interface
type UserService struct {
    config *config.BaseConfig
}

func (s *UserService) Start(ctx context.Context) error {
    log.Printf("UserService started")
    return nil
}

func (s *UserService) Stop(ctx context.Context) error {
    log.Printf("UserService stopping gracefully")
    return nil
}

func (s *UserService) HealthChecks() []health.Check {
    return []health.Check{
        health.NewAlwaysHealthyCheck("user-service"),
    }
}

func main() {
    // Load configuration
    cfg := config.DefaultBaseConfig()
    cfg.ServiceName = "user-service"
    cfg.AppEnv = "development"

    // Validate configuration
    if err := cfg.Validate(); err != nil {
        log.Fatalf("Configuration validation failed: %v", err)
    }

    // Create application
    app, err := framework.New(
        framework.WithConfig(&cfg),
        framework.WithVersion("1.0.0"),
        framework.WithComponent(&UserService{config: &cfg}),
        framework.WithHealthContributor(&UserService{config: &cfg}),
    )
    if err != nil {
        log.Fatalf("Failed to create application: %v", err)
    }

    // Run the service
    log.Printf("Starting %s...", cfg.ServiceName)
    if err := app.Run(context.Background()); err != nil {
        log.Fatalf("Application failed: %v", err)
    }
}
```

## Running Your Service

```bash
go run main.go
```

Your service will start with:
- **HTTP health endpoints** on `:8081`
  - `http://localhost:8081/health` - Overall health
  - `http://localhost:8081/health/ready` - Readiness probe
  - `http://localhost:8081/health/live` - Liveness probe
  - `http://localhost:8081/metrics` - Prometheus metrics (when the prometheus bundle is added)
- **gRPC server** on `:8080` — started only when you register a gRPC service with `framework.WithGRPCRegistrar(...)`. The example above is HTTP-only, so no gRPC server runs.

## Adding Database Connectivity

Add PostgreSQL with automatic health checks:

```go
import (
    "github.com/datariot/forge/bundles/postgresql"
)

func main() {
    // Configure database
    dbConfig := postgresql.Config{
        DatabaseURL: "postgres://user:pass@localhost:5432/mydb?sslmode=require",
        MaxOpenConns: 25,
        MaxIdleConns: 10,
        ConnMaxLifetime: 30 * time.Minute,
    }

    pgBundle := postgresql.NewBundle(dbConfig)

    app, err := framework.New(
        framework.WithConfig(&cfg),
        framework.WithBundle(pgBundle), // Add database bundle
        framework.WithComponent(service),
    )
}
```

## Adding Caching

Add Redis for caching and messaging:

```go
import (
    "github.com/datariot/forge/bundles/redis"
)

func main() {
    // Configure Redis
    redisConfig := redis.Config{
        RedisURL: "redis://localhost:6379/0",
        PoolSize: 10,
        MinIdleConns: 2,
    }

    redisBundle := redis.NewBundle(redisConfig)

    app, err := framework.New(
        framework.WithConfig(&cfg),
        framework.WithBundle(redisBundle), // Add caching bundle
        framework.WithComponent(service),
    )

    // Use Redis in your service
    cache := redisBundle.Cache()
    cache.Set(ctx, "user:123", userData, 1*time.Hour)
}
```

## Adding Authentication

Secure service-to-service communication with JWT:

```go
import (
    "github.com/datariot/forge/bundles/jwt"
)

func main() {
    // Configure JWT authentication
    jwtConfig := jwt.Config{
        SecretKey: []byte("your-32-plus-character-secret-key"),
        Issuer: "auth-service",
        Audience: "my-services",
        ServiceName: cfg.ServiceName,
    }

    jwtBundle := jwt.NewBundle(jwtConfig)

    app, err := framework.New(
        framework.WithConfig(&cfg),
        framework.WithBundle(jwtBundle), // Add authentication
        // Register BOTH interceptors — the unary one does not cover streaming RPCs.
        framework.WithUnaryInterceptor(jwtBundle.UnaryServerInterceptor()),
        framework.WithStreamInterceptor(jwtBundle.StreamServerInterceptor()),
        framework.WithComponent(service),
    )
}
```

## Environment Configuration

Configure your service using environment variables:

```bash
# Service configuration
export SERVICE_NAME="user-service"
export APP_ENV="production"
export GRPC_ADDR=":8080"
export HTTP_ADDR=":8081"
export LOG_LEVEL="info"

# Database configuration
export DATABASE_URL="postgres://user:pass@db-host:5432/mydb?sslmode=require"

# Redis configuration
export REDIS_URL="redis://redis-host:6379/0"

# JWT authentication
export JWT_SECRET="your-production-secret-key-32-characters-minimum"
export JWT_ISSUER="auth-service"
export JWT_AUDIENCE="production-services"

# Run your service
go run main.go
```

## Next Steps

- **[Explore Bundles](bundles.html)** - Learn about available integrations
- **[View Examples](https://github.com/datariot/forge/tree/main/examples)** - Complete working services
- **[API Reference](https://pkg.go.dev/github.com/datariot/forge)** - Full godoc on pkg.go.dev

## Key Concepts

### Components

Components are your business logic that implement the `Component` interface:

```go
type Component interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}
```

### Bundles

Bundles are pre-built integrations that add functionality to your application:

```go
type Bundle interface {
    Name() string
    Initialize(app *App) error
    Stop(ctx context.Context) error
}
```

### Health Checks

Implement health checks for monitoring and alerting:

```go
type HealthContributor interface {
    HealthChecks() []health.Check
}
```

### Application Lifecycle

Forge handles sophisticated startup and shutdown:

1. Initialize configuration and logging
2. Initialize bundles (PostgreSQL, Redis, etc.)
3. Start gRPC and HTTP servers
4. Start your components
5. Mark service as ready
6. Handle graceful shutdown in reverse order