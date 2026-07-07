// Package forge provides a batteries-included Go framework for building production-ready microservices.
//
// Forge is inspired by DropWizard but designed specifically for Go's strengths - interfaces,
// goroutines, and clean composition. It provides opinionated defaults while maintaining
// flexibility through a pluggable architecture.
//
// # Features
//
//   - Clean Architecture with component-based design
//   - Built-in observability (OpenTelemetry tracing & metrics)
//   - Comprehensive health checks (liveness/readiness)
//   - Graceful lifecycle management with sophisticated shutdown
//   - Environment-based configuration with validation
//   - Structured logging with development/production modes
//   - Bundle system for reusable integrations
//
// # Quick Start
//
// Create a service by implementing the Component interface:
//
//	type MyService struct {
//		db *sql.DB
//	}
//
//	func (s *MyService) Start(ctx context.Context) error {
//		// Initialize your service
//		return s.db.PingContext(ctx)
//	}
//
//	func (s *MyService) Stop(ctx context.Context) error {
//		// Clean up resources
//		return s.db.Close()
//	}
//
//	func (s *MyService) HealthChecks() []health.Check {
//		return []health.Check{
//			health.NewAlwaysHealthyCheck("my-service"),
//		}
//	}
//
// Configure and run your application:
//
//	func main() {
//		cfg := config.DefaultBaseConfig()
//		cfg.ServiceName = "my-service"
//
//		service := &MyService{}
//
//		app, err := framework.New(
//			framework.WithConfig(&cfg),
//			framework.WithVersion("1.0.0"),
//			framework.WithComponent(service),
//			framework.WithHealthContributor(service),
//		)
//		if err != nil {
//			log.Fatal(err)
//		}
//
//		if err := app.Run(context.Background()); err != nil {
//			log.Fatal(err)
//		}
//	}
//
// # Architecture
//
// Forge follows Clean Architecture principles with these key packages:
//
//   - framework: Core application lifecycle and interfaces
//   - config: Environment-based configuration management
//   - health: Kubernetes-compatible health check system
//   - forgeerrors: Structured error handling with classification
//
// # HTTP Endpoints
//
// Forge automatically provides these HTTP endpoints:
//
//   - :8080 - gRPC server (configurable via BaseConfig.GRPCAddr; only started
//     when gRPC registrars are configured)
//   - :8081/health - Combined health status
//   - :8081/health/live - Liveness probe
//   - :8081/health/ready - Readiness probe
//
// # Configuration
//
// BaseConfig provides sensible defaults; set fields in code, or add the
// configloader bundle to load YAML files with environment variable overrides:
//
//	SERVICE_NAME=my-service
//	APP_ENV=production
//	GRPC_ADDR=:8080
//	HTTP_ADDR=:8081
//	LOG_LEVEL=info
//	OTEL_ENDPOINT=https://otlp.example.com
//
// # Examples
//
// See the examples/simple-service directory for a complete working example.
//
// # More Information
//
// Visit https://github.com/datariot/forge for documentation, examples, and contribution guidelines.
package forge
