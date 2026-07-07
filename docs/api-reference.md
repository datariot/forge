---
layout: page
title: API Reference
permalink: /api-reference/
---

# API Reference

Complete API documentation for the Forge framework.

## Core Framework

### Application (`framework.App`)

The main application struct that orchestrates the entire microservice lifecycle.

```go
app, err := framework.New(options ...AppOption)
```

**Configuration Options:**

- `WithConfig(*config.BaseConfig)` - Set base configuration
- `WithVersion(string)` - Set service version
- `WithComponent(Component)` - Add business logic component
- `WithBundle(Bundle)` - Add integration bundle
- `WithGRPCRegistrar(Registrar)` - Add gRPC service
- `WithHealthContributor(HealthContributor)` - Add health checks
- `WithUnaryInterceptor(grpc.UnaryServerInterceptor)` - Add gRPC interceptor
- `WithStartupHook(StartupHook)` - Add startup hook
- `WithShutdownHook(ShutdownHook)` - Add shutdown hook

**Methods:**

- `Run(context.Context) error` - Start application and block until shutdown
- `Start(context.Context) error` - Start application components
- `Stop(context.Context) error` - Gracefully stop application
- `Config() *config.BaseConfig` - Get application configuration
- `HealthRegistry() *health.Registry` - Get health check registry
- `IsRunning() bool` - Check if application is running

### Interfaces

#### Component

Your business logic implements this interface:

```go
type Component interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}
```

#### Bundle

Pre-built integrations implement this interface:

```go
type Bundle interface {
    Name() string
    Initialize(app *App) error
}
```

#### HealthContributor

Components providing health checks implement this interface:

```go
type HealthContributor interface {
    HealthChecks() []health.Check
}
```

#### Registrar

gRPC services implement this interface:

```go
type Registrar interface {
    RegisterGRPC(server *grpc.Server) error
}
```

## Configuration (`config`)

### BaseConfig

Standard configuration for all Forge services:

```go
type BaseConfig struct {
    // Service identification
    ServiceName string `yaml:"service_name" env:"SERVICE_NAME"`
    AppEnv      string `yaml:"app_env" env:"APP_ENV"`

    // Server configuration
    GRPCAddr string `yaml:"grpc_addr" env:"GRPC_ADDR"`
    HTTPAddr string `yaml:"http_addr" env:"HTTP_ADDR"`

    // Logging
    LogLevel string `yaml:"log_level" env:"LOG_LEVEL"`

    // Timeouts
    ShutdownTimeout       time.Duration `yaml:"shutdown_timeout" env:"SHUTDOWN_TIMEOUT"`
    ReadinessInitialDelay time.Duration `yaml:"readiness_initial_delay" env:"READINESS_INITIAL_DELAY"`

    // Infrastructure URLs
    DatabaseURL string `yaml:"database_url" env:"DATABASE_URL"`
    RedisURL    string `yaml:"redis_url" env:"REDIS_URL"`

    // Features
    EnablePprof      bool `yaml:"enable_pprof" env:"ENABLE_PPROF"`
    EnableReflection bool `yaml:"enable_reflection" env:"ENABLE_REFLECTION"`
    EnableMetrics    bool `yaml:"enable_metrics" env:"ENABLE_METRICS"`
}
```

**Methods:**

- `Validate() error` - Validate configuration
- `IsDevelopment() bool` - Check if development environment
- `IsProduction() bool` - Check if production environment
- `ShouldEnableReflection() bool` - Check if gRPC reflection should be enabled

## Health Checks (`health`)

### Check Interface

Health checks implement this interface:

```go
type Check interface {
    Name() string
    Liveness(ctx context.Context) error
    Readiness(ctx context.Context) error
}
```

### Registry

Health check registry manages all health checks:

```go
registry := health.NewRegistry(logger)
registry.Register(check, config)
```

**Methods:**

- `Register(Check, CheckConfig) error` - Register health check
- `CheckLiveness(context.Context) Report` - Check liveness
- `CheckReadiness(context.Context) Report` - Check readiness
- `CheckHealth(context.Context) Report` - Check overall health

### Built-in Checks

- `NewAlwaysHealthyCheck(name)` - Always reports healthy
- `NewAlwaysUnhealthyCheck(name, err)` - Always reports unhealthy
- `NewBasicCheck(config, liveness, readiness)` - Custom check functions

## Error Handling (`forgeerrors`)

### DomainError

Structured errors with context:

```go
type DomainError struct {
    Code    string `json:"code"`
    Message string `json:"message"`
    Cause   error  `json:"cause,omitempty"`
}
```

**Methods:**

- `WithMessage(format, args...) DomainError` - Add context message
- `WithCause(error) DomainError` - Add underlying cause
- `Unwrap() error` - For error unwrapping

### Predefined Errors

```go
var (
    ErrInvalidConfiguration   = DomainError{Code: "INVALID_CONFIGURATION", ...}
    ErrRepositoryUnavailable = DomainError{Code: "REPOSITORY_UNAVAILABLE", ...}
    ErrAuthenticationFailed  = DomainError{Code: "AUTHENTICATION_FAILED", ...}
    ErrServiceUnavailable    = DomainError{Code: "SERVICE_UNAVAILABLE", ...}
    // ... more predefined errors
)
```

### Error Classification

- `IsTransientError(error) bool` - Check if error is retryable
- `IsAuthenticationError(error) bool` - Check if authentication related
- `IsValidationError(error) bool` - Check if validation related
- `IsConfigurationError(error) bool` - Check if configuration related

## Bundle APIs

### PostgreSQL Bundle

```go
import "github.com/datariot/forge/bundles/postgresql"

bundle := postgresql.NewBundle(postgresql.Config{
    DatabaseURL:        "postgres://...",
    MaxOpenConns:       25,
    MaxIdleConns:       10,
    ConnMaxLifetime:    30 * time.Minute,
    HealthCheckTimeout: 5 * time.Second,
})

db := bundle.DB() // Get *sql.DB instance
```

### Redis Bundle

```go
import "github.com/datariot/forge/bundles/redis"

bundle := redis.NewBundle(redis.Config{
    RedisURL: "redis://localhost:6379/0",
    PoolSize: 10,
})

// High-level interfaces
cache := bundle.Cache()
pubsub := bundle.PubSub()
lock := bundle.NewDistributedLock("resource", 30*time.Second)
limiter := bundle.NewRateLimiter("api")

// Cache operations
cache.Set(ctx, "key", value, 1*time.Hour)
cache.Get(ctx, "key", &dest)
cache.Delete(ctx, "key1", "key2")

// Pub/sub operations
subscription := pubsub.Subscribe(ctx, "channel")
defer subscription.Close()
pubsub.Publish(ctx, "channel", message)

// Distributed locking
acquired, err := lock.TryLock(ctx)
if acquired {
    defer lock.Unlock(ctx)
    // Critical section
}

// Rate limiting
allowed, err := limiter.Allow(ctx, userID, 100, 1*time.Minute)
```

### JWT Authentication Bundle

```go
import "github.com/datariot/forge/bundles/jwt"

bundle := jwt.NewBundle(jwt.Config{
    SecretKey:     []byte("secret"),
    Issuer:        "auth-service",
    Audience:      "services",
    TokenDuration: 1 * time.Hour,
})

// Server-side authentication
app, err := framework.New(
    framework.WithUnaryInterceptor(bundle.UnaryServerInterceptor()),
)

// Client-side token injection
conn, err := grpc.Dial("target:8080",
    grpc.WithUnaryInterceptor(bundle.UnaryClientInterceptor()),
)

// HTTP middleware
mux.Handle("/api/", bundle.HTTPMiddleware(protectedHandler))

// Permission-based protection
mux.Handle("/admin/", bundle.RequirePermissions("admin")(adminHandler))

// Access claims in handlers
claims := jwt.ClaimsFromContext(ctx)
serviceID := jwt.ServiceID(ctx)
hasPermission := jwt.HasPermission(ctx, "read:users")
```

### HTTP Client Bundle

```go
import "github.com/datariot/forge/bundles/httpclient"

bundle := httpclient.NewBundle(httpclient.Config{
    BaseURL: "https://api.example.com",
    Timeout: 30 * time.Second,
    RetryConfig: httpclient.RetryConfig{
        MaxRetries: 3,
        InitialInterval: 100 * time.Millisecond,
    },
})

client := bundle.Client()

// Type-safe requests
var user User
err := client.Get(ctx, "/users/123", &user)

var response CreateResponse
err := client.Post(ctx, "/users", createRequest, &response)

// Raw requests
resp, err := client.RawRequest(ctx, "GET", "/custom", headers, body)

// Circuit breaker monitoring
state := client.CircuitBreakerState()
counts := client.CircuitBreakerCounts()
```

### Prometheus Bundle

```go
import "github.com/datariot/forge/bundles/prometheus"

bundle := prometheus.NewBundle(prometheus.Config{
    Namespace:            "myservice",
    EnableDefaultMetrics: true,
    EnableHTTPMetrics:    true,
})

// Custom metrics
counter, err := bundle.CreateCustomCounter(
    "operations_total",
    "Total operations",
    []string{"type", "status"},
)

histogram, err := bundle.CreateCustomHistogram(
    "duration_seconds",
    "Operation duration",
    []string{"operation"},
)

// Record metrics
bundle.RecordHTTPRequest("GET", "/api/users", 200, duration)
bundle.RecordGRPCRequest("UserService.GetUser", "OK", duration)
bundle.RecordHealthCheck("database", "readiness", true, duration)

// Get metrics handler
handler := bundle.MetricsHandler()
```

### Configuration Loading Bundle

```go
import "github.com/datariot/forge/bundles/configloader"

bundle := configloader.NewBundle(configloader.Config{
    ConfigPaths: []string{"./config.yaml"},
    EnvPrefix: "MYSERVICE",
    WatchFiles: true,
})

// Load configuration
var cfg MyServiceConfig
result, err := bundle.Loader().Load(&cfg)

// Hot reload
bundle.Loader().OnConfigChange(func(newConfig interface{}) {
    // Handle configuration changes
})

// Generic loading
cfg, result, err := configloader.LoadConfig[MyServiceConfig]("./config.yaml")
```

## HTTP Endpoints

All Forge services automatically expose:

| Endpoint | Purpose | Method |
|----------|---------|--------|
| `/health` | Overall health status | GET |
| `/health/ready` | Readiness probe | GET |
| `/health/live` | Liveness probe | GET |
| `/metrics` | Prometheus metrics | GET |
| `/debug/pprof/*` | Debug endpoints (dev only) | GET |

## Environment Variables

Standard environment variables for all services:

| Variable | Purpose | Default |
|----------|---------|---------|
| `SERVICE_NAME` | Service identifier | forge-service |
| `APP_ENV` | Environment (development/staging/production) | development |
| `GRPC_ADDR` | gRPC server address | :8080 |
| `HTTP_ADDR` | HTTP server address | :8081 |
| `LOG_LEVEL` | Logging level | info |
| `SHUTDOWN_TIMEOUT` | Graceful shutdown timeout | 30s |
| `DATABASE_URL` | PostgreSQL connection string | - |
| `REDIS_URL` | Redis connection string | - |
| `JWT_SECRET` | JWT signing secret | - |
| `ENABLE_PPROF` | Enable debug endpoints | false |
| `ENABLE_REFLECTION` | Enable gRPC reflection | false (true in dev) |
| `ENABLE_METRICS` | Enable metrics collection | true |

## Error Codes

Standard error codes across all Forge services:

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `INVALID_CONFIGURATION` | 500 | Configuration validation failed |
| `REPOSITORY_UNAVAILABLE` | 503 | Database/cache unavailable |
| `AUTHENTICATION_FAILED` | 401 | Authentication required |
| `INVALID_CREDENTIAL` | 401 | Invalid or expired credential |
| `INSUFFICIENT_PERMISSIONS` | 403 | Missing required permissions |
| `SERVICE_UNAVAILABLE` | 503 | External service unavailable |
| `RATE_LIMIT_EXCEEDED` | 429 | Rate limit exceeded |

## Examples Repository

Complete examples are available in the [examples directory](https://github.com/datariot/forge/tree/main/examples):

- `simple-service/` - Basic framework usage
- `postgresql-service/` - Database integration
- `redis-service/` - Caching and messaging
- `jwt-service/` - Authentication patterns
- `httpclient-service/` - Service communication
- `prometheus-service/` - Metrics and observability
- `config-service/` - Configuration management