---
layout: page
title: API Reference
permalink: /api-reference/
---

# API Reference

The complete, always-current API reference is generated from the source and hosted on pkg.go.dev:

## → [pkg.go.dev/github.com/datariot/forge](https://pkg.go.dev/github.com/datariot/forge)

Rather than duplicate godoc here (where it drifts out of date), this page orients you to the packages. Follow the links for full type and method documentation.

## Packages

| Package | Purpose | Reference |
|---------|---------|-----------|
| `framework` | Application lifecycle, the `App` builder, `Component`/`Bundle`/`HealthContributor` interfaces, functional options | [godoc](https://pkg.go.dev/github.com/datariot/forge/framework) |
| `config` | `BaseConfig`, environment-aware defaults, validation | [godoc](https://pkg.go.dev/github.com/datariot/forge/config) |
| `health` | `Report`, `Check`, `Registry`, liveness/readiness model | [godoc](https://pkg.go.dev/github.com/datariot/forge/health) |
| `forgeerrors` | Structured `DomainError` with codes and classification | [godoc](https://pkg.go.dev/github.com/datariot/forge/forgeerrors) |
| `testutil` | Test helpers for framework consumers | [godoc](https://pkg.go.dev/github.com/datariot/forge/testutil) |
| `bundles/postgresql` | Pooled PostgreSQL via pgx | [godoc](https://pkg.go.dev/github.com/datariot/forge/bundles/postgresql) |
| `bundles/redis` | Cache, pub/sub, locks, rate limiting | [godoc](https://pkg.go.dev/github.com/datariot/forge/bundles/redis) |
| `bundles/jwt` | Service-to-service JWT auth | [godoc](https://pkg.go.dev/github.com/datariot/forge/bundles/jwt) |
| `bundles/httpclient` | Resilient HTTP client | [godoc](https://pkg.go.dev/github.com/datariot/forge/bundles/httpclient) |
| `bundles/prometheus` | Metrics + automatic request instrumentation | [godoc](https://pkg.go.dev/github.com/datariot/forge/bundles/prometheus) |
| `bundles/configloader` | Multi-source config with hot reload | [godoc](https://pkg.go.dev/github.com/datariot/forge/bundles/configloader) |

## Core building blocks

Your service implements these framework interfaces:

```go
// Business logic — started in registration order, stopped in reverse.
type Component interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}

// Optional: contribute health checks.
type HealthContributor interface {
    HealthChecks() []health.Check
}

// Optional: register gRPC services.
type Registrar interface {
    RegisterGRPC(server *grpc.Server) error
}
```

And compose the application with functional options:

```go
app, err := framework.New(
    framework.WithConfig(&cfg),
    framework.WithVersion("1.0.0"),
    framework.WithComponent(svc),
    framework.WithBundle(postgresql.NewBundle(dbConfig)),
    framework.WithHealthContributor(svc),
    framework.WithGRPCRegistrar(svc), // starts the gRPC server
)
```

See [Getting Started](getting-started.html) for a full walkthrough and [Bundles](bundles.html) for per-integration configuration.
