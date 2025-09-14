# Simple Service Example

This example demonstrates how to create a basic service using the Forge framework.

## Features Demonstrated

- Configuration management with environment variable support
- Component lifecycle management (Start/Stop)
- Health checks integration
- Graceful shutdown handling
- Structured logging
- OpenTelemetry tracing (when OTEL endpoint is configured)

## Running the Example

```bash
# Run with default configuration
go run main.go

# Run with custom configuration
SERVICE_NAME=my-service MESSAGE="Custom greeting!" go run main.go

# Run with custom ports
GRPC_ADDR=:9090 HTTP_ADDR=:9091 go run main.go
```

## Health Checks

Once running, you can check the service health:

```bash
# Overall health
curl http://localhost:8081/health

# Readiness check
curl http://localhost:8081/health/ready

# Liveness check
curl http://localhost:8081/health/live
```

## Configuration

The service can be configured via environment variables:

- `SERVICE_NAME`: Name of the service (default: "simple-service")
- `MESSAGE`: Custom message (default: "Hello from Forge!")
- `LOG_LEVEL`: Logging level (debug, info, warn, error)
- `GRPC_ADDR`: gRPC server address (default: ":8080")
- `HTTP_ADDR`: HTTP health server address (default: ":8081")
- `APP_ENV`: Environment (development, staging, production)

## Shutdown

The service responds gracefully to SIGINT (Ctrl+C) and SIGTERM signals, allowing all components to clean up properly.