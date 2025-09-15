---
layout: page
title: Bundles
permalink: /bundles/
---

# Forge Bundles

Bundles are pre-built integrations that add functionality to your Forge application. Each bundle provides production-ready capabilities with security hardening and comprehensive observability.

## Available Bundles

### 🗄️ [PostgreSQL Bundle](bundles/postgresql/)

Production-ready PostgreSQL integration with connection pooling and health checks.

```go
import "github.com/datariot/forge/bundles/postgresql"

pgBundle := postgresql.NewBundle(postgresql.Config{
    DatabaseURL: "postgres://user:pass@localhost:5432/mydb?sslmode=require",
    MaxOpenConns: 25,
    MaxIdleConns: 10,
})

app, err := framework.New(
    framework.WithBundle(pgBundle),
)
```

**Features:**
- Connection pool management with configurable limits
- Health checks for database connectivity
- Transaction utilities and patterns
- Graceful connection lifecycle management
- Production-ready security validation

### 🚀 [Redis Bundle](bundles/redis/)

Comprehensive Redis integration for caching, messaging, and distributed operations.

```go
import "github.com/datariot/forge/bundles/redis"

redisBundle := redis.NewBundle(redis.Config{
    RedisURL: "redis://localhost:6379/0",
    PoolSize: 10,
})

// Use high-level interfaces
cache := redisBundle.Cache()
pubsub := redisBundle.PubSub()
lock := redisBundle.NewDistributedLock("resource", 30*time.Second)
```

**Features:**
- High-level caching interface with TTL support
- Pub/sub messaging with managed subscriptions
- Distributed locking with secure random values
- Rate limiting with sliding window algorithm
- Connection pool monitoring and health checks

### 🔐 [JWT Authentication Bundle](bundles/jwt/)

Secure service-to-service authentication with automatic token management.

```go
import "github.com/datariot/forge/bundles/jwt"

jwtBundle := jwt.NewBundle(jwt.Config{
    SecretKey: []byte("your-32-plus-character-secret-key"),
    Issuer: "auth-service",
    Audience: "my-services",
})

app, err := framework.New(
    framework.WithBundle(jwtBundle),
    framework.WithUnaryInterceptor(jwtBundle.UnaryServerInterceptor()),
)
```

**Features:**
- Automatic gRPC authentication interceptors
- HTTP middleware for endpoint protection
- Token caching and lifecycle management
- Service identity and permissions model
- Path-based authentication exemptions

### 🌐 [HTTP Client Bundle](bundles/httpclient/)

Resilient HTTP client with circuit breakers, retries, and authentication.

```go
import "github.com/datariot/forge/bundles/httpclient"

httpBundle := httpclient.NewBundle(httpclient.Config{
    BaseURL: "https://api.example.com",
    Timeout: 30 * time.Second,
    RetryConfig: httpclient.RetryConfig{
        MaxRetries: 3,
        InitialInterval: 100 * time.Millisecond,
    },
})

client := httpBundle.Client()
var response UserResponse
err := client.Get(ctx, "/users/123", &response)
```

**Features:**
- Circuit breaker protection against failing services
- Exponential backoff retry logic with jitter
- Automatic JWT token injection
- Request/response logging with security filtering
- Connection pool optimization

### 📊 [Prometheus Bundle](bundles/prometheus/)

Comprehensive metrics collection with pre-built Grafana dashboards.

```go
import "github.com/datariot/forge/bundles/prometheus"

promBundle := prometheus.NewBundle(prometheus.Config{
    Namespace: "myservice",
    EnableHTTPMetrics: true,
    EnableGRPCMetrics: true,
})

// Create custom metrics
counter, err := promBundle.CreateCustomCounter(
    "users_created_total",
    "Total users created",
    []string{"source"},
)
```

**Features:**
- Automatic HTTP/gRPC request metrics
- Health check success/failure tracking
- Database and Redis connection monitoring
- JWT authentication metrics
- Custom metrics with consistent labeling
- Pre-built Grafana dashboard templates

### ⚙️ [Configuration Loading Bundle](bundles/configloader/)

Automatic configuration loading from files and environment variables.

```go
import "github.com/datariot/forge/bundles/configloader"

configBundle := configloader.NewBundle(configloader.Config{
    ConfigPaths: []string{"./config.yaml"},
    EnvPrefix: "MYSERVICE",
    WatchFiles: true, // Hot reload in development
})

var cfg MyServiceConfig
result, err := configBundle.Loader().Load(&cfg)
```

**Features:**
- Multi-source configuration loading (files, env, defaults)
- YAML and JSON format support
- Environment variable binding with struct tags
- Hot reload with file watching (development)
- Secure handling of sensitive configuration data
- Configuration validation and debugging

## Bundle Integration Patterns

### Multiple Bundles

Combine bundles for complete functionality:

```go
app, err := framework.New(
    framework.WithConfig(&cfg),
    framework.WithBundle(pgBundle),      // Database
    framework.WithBundle(redisBundle),   // Caching
    framework.WithBundle(jwtBundle),     // Authentication
    framework.WithBundle(httpBundle),    // HTTP client
    framework.WithBundle(promBundle),    // Metrics
    framework.WithComponent(myService),
)
```

### Bundle Health Checks

Bundles automatically contribute health checks:

```go
// PostgreSQL health check: database connectivity
// Redis health check: cache connectivity
// JWT health check: token validation
// HTTP client health check: external service connectivity
// Prometheus health check: metrics collection

// Access at:
// GET /health - Combined health status
// GET /health/ready - Readiness probe
// GET /health/live - Liveness probe
```

### Metrics Integration

Bundles automatically integrate with Prometheus:

```go
// Automatic metrics from bundles:
// - Database connection pool metrics
// - Redis connection and operation metrics
// - JWT token validation metrics
// - HTTP client request metrics
// - Health check success/failure rates

// View at: http://localhost:8081/metrics
```

## Production Configuration

### Environment Variables

All bundles support environment variable configuration:

```bash
# Framework configuration
export SERVICE_NAME="user-service"
export APP_ENV="production"
export LOG_LEVEL="warn"

# Database configuration
export DATABASE_URL="postgres://user:pass@prod-db:5432/mydb?sslmode=require"

# Redis configuration
export REDIS_URL="rediss://user:pass@prod-redis:6380/0"

# JWT configuration
export JWT_SECRET="production-secret-key-32-characters-minimum"
export JWT_ISSUER="production-auth-service"
export JWT_AUDIENCE="production-services"

# Security configuration
export ENABLE_PPROF="false"
export ENABLE_REFLECTION="false"
export ENABLE_CORS="false"
```

### Docker Deployment

```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o myservice .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/myservice .
EXPOSE 8080 8081
CMD ["./myservice"]
```

### Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: user-service
spec:
  replicas: 3
  selector:
    matchLabels:
      app: user-service
  template:
    metadata:
      labels:
        app: user-service
    spec:
      containers:
      - name: user-service
        image: myservice:latest
        ports:
        - containerPort: 8080  # gRPC
        - containerPort: 8081  # HTTP/Health
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8081
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 5
        env:
        - name: SERVICE_NAME
          value: "user-service"
        - name: APP_ENV
          value: "production"
        - name: DATABASE_URL
          valueFrom:
            secretKeyRef:
              name: db-secret
              key: url
```

## Next Steps

- **[Bundle Reference](bundles/)** - Detailed bundle documentation
- **[Examples](examples/)** - Complete working examples
- **[API Reference](api-reference/)** - Framework API documentation