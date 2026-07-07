# Forge

[![CI](https://github.com/datariot/forge/actions/workflows/test.yml/badge.svg)](https://github.com/datariot/forge/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/datariot/forge.svg)](https://pkg.go.dev/github.com/datariot/forge)
[![Go Report Card](https://goreportcard.com/badge/github.com/datariot/forge)](https://goreportcard.com/report/github.com/datariot/forge)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
![Go 1.25+](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go)

A batteries-included Go framework for building production-ready microservices — inspired by DropWizard, designed for Go's strengths.

Forge wires up the boilerplate every service re-implements — lifecycle orchestration, health checks, structured logging, tracing, metrics, graceful shutdown — behind a small, composable API, so you write business logic instead of plumbing.

```go
app, err := framework.New(
    framework.WithConfig(&cfg),
    framework.WithComponent(myService),
    framework.WithBundle(postgresql.NewBundle(dbConfig)),
)
if err != nil {
    log.Fatal(err)
}
log.Fatal(app.Run(context.Background()))
```

## Why Forge

**Use Forge when** you're running more than one Go service and are tired of copy-pasting the same `main.go` — the health endpoints, the OTel setup, the signal handling, the "did I remember to drain connections on shutdown" checklist. Forge gives you opinionated, production-grade defaults for all of it and a bundle system to add PostgreSQL, Redis, JWT auth, a resilient HTTP client, and Prometheus metrics with one line each.

**What you get out of the box:**

- **Lifecycle orchestration** — ordered startup, reverse-order graceful shutdown, timeout handling, and startup/shutdown hooks.
- **Health checks** — Kubernetes-style `/health`, `/health/ready`, `/health/live` plus a gRPC health service, run on a background schedule and cached so a slow dependency can't stall your probes.
- **Observability** — OpenTelemetry tracing (W3C propagation, configurable sampling), structured JSON logging via zerolog, and automatic HTTP/gRPC request metrics when the Prometheus bundle is added.
- **Bundles** — PostgreSQL (pooled, via pgx), Redis (cache, pub/sub, distributed locks, rate limiting), JWT service auth (unary + stream interceptors), a circuit-breaking HTTP client, Prometheus, and multi-source config loading with hot reload.
- **Config with validation** — a common `BaseConfig` with environment-aware defaults; layer YAML files and environment overrides via the configloader bundle.

**Forge is probably not for you if** you want a full web framework with routing, middleware, and an ORM (reach for Echo/Gin/Fiber + your data layer), or if your service is a single small binary where the framework's structure is more than you need. Forge is opinionated about *operability*, not about how you write handlers.

**Status:** pre-1.0. The API is settling and may still change between minor versions; pin your dependency and read the [CHANGELOG](CHANGELOG.md) before upgrading.

## Install

```bash
go get github.com/datariot/forge
```

Requires Go 1.25+.

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

type Greeter struct{}

func (Greeter) Start(ctx context.Context) error { log.Println("greeter up"); return nil }
func (Greeter) Stop(ctx context.Context) error  { log.Println("greeter down"); return nil }

func (Greeter) HealthChecks() []health.Check {
    return []health.Check{health.NewAlwaysHealthyCheck("greeter")}
}

func main() {
    cfg := config.DefaultBaseConfig()
    cfg.ServiceName = "greeter"

    svc := Greeter{}
    app, err := framework.New(
        framework.WithConfig(&cfg),
        framework.WithVersion("1.0.0"),
        framework.WithComponent(svc),
        framework.WithHealthContributor(svc),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Run blocks until SIGINT/SIGTERM, then shuts down gracefully.
    if err := app.Run(context.Background()); err != nil {
        log.Fatal(err)
    }
}
```

Run it, then hit the health endpoints on the HTTP server (`:8081` by default):

```bash
curl localhost:8081/health        # overall status
curl localhost:8081/health/ready  # readiness probe
curl localhost:8081/health/live   # liveness probe
```

The gRPC server (`:8080` by default) starts only when you register a gRPC service with `framework.WithGRPCRegistrar(...)`, so an HTTP-only service like the one above runs without it.

See [`examples/`](examples/) for complete services using each bundle, and the [Getting Started guide](docs/getting-started.md) for a step-by-step walkthrough.

## Bundles

Add a bundle with `framework.WithBundle(...)`; it self-initializes during startup, contributes its own health checks, and cleans up on shutdown.

| Bundle | What it provides |
|--------|------------------|
| `bundles/postgresql` | Pooled `database/sql` access via pgx, with a readiness check |
| `bundles/redis` | Cache, pub/sub, distributed locks, sliding-window rate limiting |
| `bundles/jwt` | Service-to-service auth with gRPC unary + stream interceptors and HTTP middleware |
| `bundles/httpclient` | HTTP client with circuit breaker, retries, backoff, and an optional host allowlist |
| `bundles/prometheus` | Metrics registry and **automatic** HTTP/gRPC request metrics |
| `bundles/configloader` | YAML + environment config loading with validation and hot reload |

Full reference: [pkg.go.dev/github.com/datariot/forge](https://pkg.go.dev/github.com/datariot/forge).

## Documentation

- [Getting Started](docs/getting-started.md) — build your first service step by step
- [Bundles Guide](docs/bundles.md) — configuring and using each integration
- [API Reference](https://pkg.go.dev/github.com/datariot/forge) — full godoc on pkg.go.dev
- [Examples](examples/) — runnable services
- [Contributing](CONTRIBUTING.md) — dev setup and PR workflow

## Development

```bash
task test           # unit tests (race-enabled)
task test:coverage  # coverage report
task lint           # gofmt check + go vet + golangci-lint
task build:all      # framework + examples
task --list         # everything else
```

`go test ./...` and `go build ./...` work without Task if you prefer. Integration tests (PostgreSQL + Redis) run under `task test:integration` and require Docker. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT — see [LICENSE](LICENSE).
