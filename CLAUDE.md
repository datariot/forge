# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## About Forge

Forge is a batteries-included Go framework for building production-ready microservices, inspired by DropWizard. It emphasizes Clean Architecture principles, component-based design, and production readiness with built-in observability, health checks, and graceful lifecycle management.

**Philosophy**: "Batteries included" but lightweight - carefully evaluate third-party dependencies before adding them.

## Common Commands

[Taskfile](https://taskfile.dev) equivalents exist for all of these (`task --list`); the raw Go commands below work without it.

### Building and Testing
```bash
# Build all packages
go build ./...

# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests for specific package
go test ./framework
go test ./bundles/postgresql

# Check for race conditions
go test -race ./...

# Run integration tests (requires Docker services)
go test -tags=integration ./...

# Run benchmarks
go test -bench=. -benchmem ./...
```

### Code Quality
```bash
# Format code (always run before committing)
go fmt ./...

# Vet code for issues
go vet ./...

# Static analysis (if golangci-lint available)
golangci-lint run
```

### Module Management
```bash
# Tidy dependencies
go mod tidy

# Download dependencies
go mod download

# Verify dependencies
go mod verify
```

### Documentation Site
```bash
# Run Jekyll docs site locally
cd docs
bundle install
bundle exec jekyll serve
# Site available at http://localhost:4000/forge/
```

## Architecture Overview

Forge implements Clean Architecture with these core concepts:

### Framework Core (`framework/`)
The heart of Forge - orchestrates the entire application lifecycle:

- **App (`app.go`)**: Central application struct managing lifecycle, component registration, bundle initialization, and graceful shutdown orchestration. The App uses a **builder pattern** with functional options (`WithConfig`, `WithComponent`, `WithBundle`, etc.)

- **Component Interface**: Business logic implements `Start(ctx)` and `Stop(ctx)` methods. Components start in registration order and stop in reverse order during shutdown.

- **Bundle Interface**: Pre-built integrations (PostgreSQL, Redis, JWT, etc.) that self-initialize with the App. Bundles implement `Initialize(app)` to set up resources and dependencies.

- **HealthContributor Interface**: Components and bundles can provide health checks by implementing `HealthChecks() []health.Check`

- **Hook System**:
  - Startup hooks: Execute after bundles init, before servers start
  - Shutdown hooks: Execute during graceful shutdown, before component stop

### Lifecycle Orchestration
The App coordinates startup/shutdown in this order:

**Startup:**
1. Initialize observability (OpenTelemetry, metrics, logging)
2. Initialize bundles (in registration order)
3. Execute startup hooks
4. Start gRPC server (port from `GRPC_ADDR`, default `:8080`; only started when gRPC registrars are configured — HTTP-only services skip it)
5. Start HTTP server for health checks (port from `HTTP_ADDR`, default `:8081`)
6. Start components (in registration order)
7. Mark service as ready

**Shutdown (reverse order with timeout handling):**
1. Mark service as not ready
2. Stop components (reverse registration order)
3. Stop HTTP server (with drain period)
4. Stop gRPC server (with graceful stop)
5. Execute shutdown hooks
6. Stop bundles (reverse order)
7. Flush observability telemetry

### Configuration (`config/`)
- **BaseConfig (`base.go`)**: Common configuration struct for all services with defaults and validation
- Supports development, staging, production environments
- BaseConfig itself carries yaml/env struct tags but does not read files or the environment; YAML file loading + environment variable overrides are provided by the `configloader` bundle. Without it, use `DefaultBaseConfig()` and set fields in code.
- Components requiring configuration should embed `BaseConfig` and add service-specific fields
- Always implement the `Validator` interface for custom config structs

### Health Checks (`health/`)
Kubernetes-compatible health check system with concurrent execution:

- **Registry (`registry.go`)**: Manages and executes health checks concurrently with timeout handling
- **Check Interface**: Components implement `Liveness(ctx)` and `Readiness(ctx)` methods
- **HTTP Endpoints** (auto-exposed on HTTP server):
  - `/health` - Combined liveness + readiness status
  - `/health/live` - Liveness probe (is the service alive?)
  - `/health/ready` - Readiness probe (ready to serve traffic?)

### Available Bundles (`bundles/`)
Pre-built integrations following the Bundle interface:

- **postgresql**: Database connection pooling, health checks, lifecycle management
- **redis**: Cache operations, pub/sub, distributed locking, rate limiting
- **jwt**: Service-to-service authentication with gRPC/HTTP interceptors
- **httpclient**: Resilient HTTP client with circuit breaker, retries, backoff
- **prometheus**: Metrics collection and integration with framework
- **configloader**: Multi-source configuration loading with hot reload

Each bundle provides:
- Self-contained initialization via `Initialize(app)`
- Health checks via `HealthContributor` interface
- Graceful cleanup in `Stop()` method
- Production-ready defaults

### Error Handling (`errors/`)
Domain error patterns and classification utilities for consistent error handling across services.

### Observability (`framework/observability.go`, `framework/logging.go`)
Built-in production observability:

- **OpenTelemetry**: Distributed tracing with configurable sampling rates
- **Structured Logging**: JSON logging via zerolog with context propagation
- **Metrics**: Ready for Prometheus integration (use prometheus bundle)
- **Health Endpoints**: Kubernetes-compatible liveness/readiness probes

## Creating a Service

Standard pattern for building a Forge service:

1. **Define Configuration**: Embed `config.BaseConfig` + service-specific fields
2. **Implement Components**: Create structs implementing `Component` interface
3. **Add Health Checks**: Optionally implement `HealthContributor` for dependencies
4. **Build Application**: Use `framework.New()` with builder options
5. **Run**: Call `app.Run(ctx)` which blocks until shutdown signal

Example structure:
```go
type ServiceConfig struct {
    config.BaseConfig `yaml:",inline"`
    // Service-specific config fields
}

type ServiceComponent struct {
    config *ServiceConfig
    // Dependencies (db, redis, etc.)
}

func (c *ServiceComponent) Start(ctx context.Context) error { /* ... */ }
func (c *ServiceComponent) Stop(ctx context.Context) error { /* ... */ }
func (c *ServiceComponent) HealthChecks() []health.Check { /* ... */ }

func main() {
    cfg := LoadConfig()
    component := NewServiceComponent(&cfg)

    app, err := framework.New(
        framework.WithConfig(&cfg.BaseConfig),
        framework.WithVersion("1.0.0"),
        framework.WithComponent(component),
        framework.WithBundle(postgresql.NewBundle(cfg.PostgreSQL)),
        framework.WithHealthContributor(component),
    )
    if err != nil {
        log.Fatal(err)
    }

    if err := app.Run(context.Background()); err != nil {
        log.Fatal(err)
    }
}
```

See `examples/` directory for complete working examples.

## Testing Strategy

**Current Status**: ~70% overall coverage; CI enforces a 70% threshold. TESTING.md's 85-90% per-package targets are aspirational — the postgresql/redis bundles sit well below them without Docker-based integration runs.

See `TESTING.md` for detailed testing plan. Key points:

- **Target Coverage**: 85%+ for bundles, 90%+ for framework core
- **Test Categories**: Unit tests, integration tests (with Docker), E2E tests, security tests, performance benchmarks
- **Test Tags**: Use `-tags=integration` for tests requiring real infrastructure
- **Test Naming**: `TestComponentName_MethodName_ExpectedBehavior`
- **Integration Tests**: Require Docker for PostgreSQL, Redis, etc.

Run example services to validate functionality:
```bash
cd examples/simple-service
go run main.go
# Check health at http://localhost:8081/health
```

## Directory Structure

```
forge/
├── framework/          # Core framework (app lifecycle, logging, observability, shutdown)
├── config/             # Configuration management (BaseConfig, validation, env detection)
├── health/             # Health check system (registry, status, concurrent execution)
├── bundles/            # Pre-built integrations (postgresql, redis, jwt, etc.)
├── errors/             # Error handling utilities
├── examples/           # Example service implementations
├── docs/               # GitHub Pages documentation site (Jekyll)
├── testutil/           # Testing utilities (assertions, test configs, zerolog test logger)
├── TESTING.md          # Comprehensive testing strategy
└── CLAUDE.md           # This file
```

## Development Guidelines

### Adding New Bundles
When creating a new bundle:

1. Implement the `Bundle` interface with `Initialize(app)` method
2. Register resources with the App during initialization
3. Provide health checks via `HealthContributor` interface
4. Implement `Stop()` for graceful cleanup
5. Follow existing bundle patterns (see `bundles/postgresql` or `bundles/redis`)
6. Evaluate third-party dependencies carefully (lightweight philosophy)
7. Add comprehensive tests (unit + integration)
8. Document in `docs/bundles.md`

### Modifying Framework Core
Framework core changes require extra care:

- Maintain backward compatibility where possible
- Update lifecycle orchestration carefully (startup/shutdown order matters)
- Add tests for new functionality (target 90%+ coverage)
- Document breaking changes clearly
- Update examples if interfaces change

### Security Considerations
- No hardcoded credentials (always use environment variables)
- Validate all configuration inputs
- Use TLS for production (gRPC and HTTP)
- Implement proper authentication/authorization (JWT bundle)
- Sanitize logs to prevent credential leakage
- Follow OWASP security guidelines

## Common Patterns

### Component Dependencies
Components receive dependencies via constructor injection:
```go
type MyComponent struct {
    db    *sql.DB
    cache redis.UniversalClient
    cfg   *Config
}

func NewMyComponent(db *sql.DB, cache redis.UniversalClient, cfg *Config) *MyComponent {
    return &MyComponent{db: db, cache: cache, cfg: cfg}
}
```

### Bundle Registration Order
Bundles initialize in registration order - put dependencies first:
```go
framework.New(
    framework.WithBundle(postgresql.NewBundle(...)),  // First
    framework.WithBundle(redis.NewBundle(...)),       // Second
    framework.WithComponent(myComponent),             // Uses both DB and Redis
)
```

### Health Check Patterns
Health checks should be fast and focused:
```go
func (c *DatabaseCheck) Liveness(ctx context.Context) error {
    return c.db.PingContext(ctx)  // Fast, lightweight
}

func (c *DatabaseCheck) Readiness(ctx context.Context) error {
    // More comprehensive check
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    var result int
    err := c.db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
    return err
}
```

### Graceful Shutdown
Components should respect context cancellation:
```go
func (c *WorkerComponent) Start(ctx context.Context) error {
    go func() {
        for {
            select {
            case <-ctx.Done():
                return  // Stop on context cancellation
            case work := <-c.workQueue:
                c.processWork(work)
            }
        }
    }()
    return nil
}
```

## Troubleshooting

### Health Check Failures
- Check logs for specific health check errors
- Verify database/redis/external service connectivity
- Check per-check timeout configuration (`CheckConfig.Timeout`)
- Use `/health/live` vs `/health/ready` to isolate issues

### Startup Issues
- Review startup logs for initialization errors
- Check configuration validation errors
- Verify bundle dependencies are registered in correct order
- Ensure environment variables are set correctly

### Shutdown Hangs
- Check component `Stop()` methods respect context timeout
- Review shutdown hooks for blocking operations
- Increase shutdown timeout if needed (configured in BaseConfig)
- Check for goroutine leaks preventing clean shutdown
