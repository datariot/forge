---
layout: page
title: Examples
permalink: /examples/
---

# Forge Examples

Complete working examples demonstrating Forge framework capabilities.

## Available Examples

### 🔧 [Simple Service](https://github.com/datariot/forge/tree/main/examples/simple-service)

A minimal Forge service demonstrating basic framework usage.

**Features:**
- Basic component implementation
- Health checks integration
- Graceful shutdown handling
- Structured logging

**Run:**
```bash
cd examples/simple-service
go run main.go
```

### 🗄️ [PostgreSQL Service](https://github.com/datariot/forge/tree/main/examples/postgresql-service)

Database-backed service with PostgreSQL integration.

**Features:**
- PostgreSQL bundle integration
- Database connection pooling
- Transaction patterns
- Database health checks
- User CRUD operations

**Prerequisites:**
```bash
# Start PostgreSQL
docker run --name postgres -e POSTGRES_PASSWORD=password -d -p 5432:5432 postgres

# Create database
createdb forge_example

# Run migrations (example)
golang-migrate -path ./migrations -database "postgres://postgres:password@localhost:5432/forge_example?sslmode=disable" up
```

**Run:**
```bash
cd examples/postgresql-service
export DATABASE_URL="postgres://postgres:password@localhost:5432/forge_example?sslmode=disable"
go run main.go
```

### 🚀 [Redis Service](https://github.com/datariot/forge/tree/main/examples/redis-service)

Caching and messaging service with Redis integration.

**Features:**
- Redis bundle integration
- Caching operations with TTL
- Pub/sub messaging
- Distributed locking
- Rate limiting
- Connection pool monitoring

**Prerequisites:**
```bash
# Start Redis
docker run --name redis -d -p 6379:6379 redis:alpine
```

**Run:**
```bash
cd examples/redis-service
export REDIS_URL="redis://localhost:6379/0"
go run main.go
```

**Test:**
```bash
# Test caching
curl -X POST http://localhost:8081/api/cache/user/123 \
  -H "Content-Type: application/json" \
  -d '{"name": "John Doe", "email": "john@example.com"}'

curl http://localhost:8081/api/cache/user/123

# Test pub/sub
curl -X POST http://localhost:8081/api/events/user.created \
  -H "Content-Type: application/json" \
  -d '{"user_id": "123", "event": "user_created"}'
```

### 🔐 [JWT Service](https://github.com/datariot/forge/tree/main/examples/jwt-service)

Authentication service demonstrating JWT security patterns.

**Features:**
- JWT bundle integration
- gRPC authentication interceptors
- HTTP middleware protection
- Service-to-service authentication
- Permission-based authorization

**Run:**
```bash
cd examples/jwt-service
export JWT_SECRET="your-very-secure-secret-key-here-32-bytes-minimum"
export JWT_ISSUER="auth-service"
export JWT_AUDIENCE="forge-services"
go run main.go
```

**Test:**
```bash
# Generate token (demo endpoint)
curl -X POST http://localhost:8081/auth/token \
  -H "Content-Type: application/json" \
  -d '{"service_id": "test-client", "permissions": ["read:users"]}'

# Use token for protected endpoint
curl -H "Authorization: Bearer <token>" http://localhost:8081/api/protected
```

### 🌐 [HTTP Client Service](https://github.com/datariot/forge/tree/main/examples/httpclient-service)

Service-to-service communication with HTTP client bundle.

**Features:**
- HTTP client bundle integration
- Circuit breaker patterns
- Retry logic with exponential backoff
- Authentication integration
- Request/response logging

**Run:**
```bash
cd examples/httpclient-service
go run main.go
```

**Test:**
```bash
# Test GET request
curl http://localhost:8081/api/test/get

# Test POST request
curl -X POST http://localhost:8081/api/test/post

# Test circuit breaker
curl http://localhost:8081/api/test/unreliable

# Check circuit breaker status
curl http://localhost:8081/api/circuit-breaker/status
```

### 📊 [Prometheus Service](https://github.com/datariot/forge/tree/main/examples/prometheus-service)

Observability service with comprehensive metrics collection.

**Features:**
- Prometheus bundle integration
- Automatic application metrics
- Custom metrics registration
- Grafana dashboard integration
- Health check metrics

**Run:**
```bash
cd examples/prometheus-service
go run main.go
```

**Test:**
```bash
# View Prometheus metrics
curl http://localhost:8081/metrics

# Generate metrics
curl http://localhost:8081/api/users
curl -X POST http://localhost:8081/api/users -d '{"name":"John","email":"john@example.com"}'

# Test error scenarios
curl http://localhost:8081/api/simulate/error
curl http://localhost:8081/api/simulate/slow
```

### ⚙️ [Configuration Service](https://github.com/datariot/forge/tree/main/examples/config-service)

Configuration management with automatic loading and hot reload.

**Features:**
- Configuration loader bundle integration
- Multi-source configuration loading
- Environment variable binding
- Hot reload capabilities
- Secure sensitive data handling

**Run:**
```bash
cd examples/config-service

# With config file
echo 'service_name: "config-demo"
database_url: "postgres://localhost:5432/demo"
debug: true' > config.yaml

go run main.go

# With environment variables (overrides file)
export DATABASE_URL="postgres://prod:5432/proddb"
export DEBUG="false"
go run main.go
```

**Test:**
```bash
# Configuration information
curl http://localhost:8081/api/config/info

# Reload configuration
curl -X POST http://localhost:8081/api/config/reload

# View configuration sources
curl http://localhost:8081/api/config/sources
```

## Production Examples

### Multi-Bundle Service

Complete service using multiple bundles:

```go
package main

import (
    "context"
    "log"

    "github.com/datariot/forge/bundles/postgresql"
    "github.com/datariot/forge/bundles/redis"
    "github.com/datariot/forge/bundles/jwt"
    "github.com/datariot/forge/bundles/httpclient"
    "github.com/datariot/forge/bundles/prometheus"
    "github.com/datariot/forge/config"
    "github.com/datariot/forge/framework"
)

func main() {
    cfg := config.DefaultBaseConfig()
    cfg.ServiceName = "production-service"
    cfg.AppEnv = "production"

    // Create bundles
    pgBundle := postgresql.NewBundle(postgresql.Config{
        DatabaseURL: os.Getenv("DATABASE_URL"),
        MaxOpenConns: 50,
    })

    redisBundle := redis.NewBundle(redis.Config{
        RedisURL: os.Getenv("REDIS_URL"),
        PoolSize: 20,
    })

    jwtBundle := jwt.NewBundle(jwt.Config{
        SecretKey: []byte(os.Getenv("JWT_SECRET")),
        Issuer: "auth-service",
        Audience: "production-services",
    })

    httpBundle := httpclient.NewBundle(httpclient.Config{
        BaseURL: "https://external-api.com",
        Timeout: 30 * time.Second,
    })

    promBundle := prometheus.NewBundle(prometheus.Config{
        Namespace: cfg.ServiceName,
        EnableDefaultMetrics: true,
        EnableHTTPMetrics: true,
    })

    // Create application with all bundles
    app, err := framework.New(
        framework.WithConfig(&cfg),
        framework.WithVersion("1.0.0"),
        framework.WithBundle(pgBundle),
        framework.WithBundle(redisBundle),
        framework.WithBundle(jwtBundle),
        framework.WithBundle(httpBundle),
        framework.WithBundle(promBundle),
        framework.WithUnaryInterceptor(jwtBundle.UnaryServerInterceptor()),
        framework.WithComponent(myService),
    )
    if err != nil {
        log.Fatal(err)
    }

    if err := app.Run(context.Background()); err != nil {
        log.Fatal(err)
    }
}
```

## Testing Examples

All examples include comprehensive testing patterns:

```bash
# Health checks
curl http://localhost:8081/health

# Metrics (if Prometheus bundle enabled)
curl http://localhost:8081/metrics

# gRPC reflection (development)
grpc_cli ls localhost:8080

# gRPC health checks
grpc_cli call localhost:8080 grpc.health.v1.Health.Check ""
```

## Deployment Examples

### Docker Compose

```yaml
version: '3.8'
services:
  user-service:
    build: .
    ports:
      - "8080:8080"
      - "8081:8081"
    environment:
      - SERVICE_NAME=user-service
      - APP_ENV=development
      - DATABASE_URL=postgres://postgres:password@postgres:5432/mydb
      - REDIS_URL=redis://redis:6379/0
    depends_on:
      - postgres
      - redis

  postgres:
    image: postgres:15-alpine
    environment:
      - POSTGRES_PASSWORD=password
      - POSTGRES_DB=mydb
    volumes:
      - postgres_data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    volumes:
      - redis_data:/data

volumes:
  postgres_data:
  redis_data:
```

### Helm Chart

```yaml
apiVersion: v2
name: forge-service
version: 0.1.0
appVersion: "1.0"

dependencies:
  - name: postgresql
    version: 12.x.x
    repository: https://charts.bitnami.com/bitnami
  - name: redis
    version: 17.x.x
    repository: https://charts.bitnami.com/bitnami
```

## Next Steps

- **[API Reference](api-reference.html)** - Detailed API documentation
- **[Production Deployment](deployment.html)** - Production best practices
- **[Contributing](https://github.com/datariot/forge/blob/main/CONTRIBUTING.md)** - How to contribute