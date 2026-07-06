// Package framework provides the core application lifecycle management for Forge microservices.
//
// The framework package implements Clean Architecture principles with a component-based design
// that enables building production-ready microservices with built-in observability, health checks,
// graceful shutdown, and lifecycle management.
//
// # Basic Usage
//
// Create a service by implementing the Component interface and using the App builder:
//
//	cfg := config.DefaultBaseConfig()
//	cfg.ServiceName = "my-service"
//
//	myComponent := &MyComponent{}
//
//	app, err := framework.New(
//		framework.WithConfig(&cfg),
//		framework.WithVersion("1.0.0"),
//		framework.WithComponent(myComponent),
//	)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	if err := app.Run(context.Background()); err != nil {
//		log.Fatal(err)
//	}
//
// # Architecture
//
// The framework follows Clean Architecture with these key concepts:
//
//   - Components: Your business logic implementing Component interface
//   - Bundles: Pre-built integrations (PostgreSQL, Redis, etc.)
//   - App: Main application orchestrating lifecycle and dependencies
//   - Health: Comprehensive health check system with liveness/readiness
//   - Observability: Built-in OpenTelemetry tracing and metrics
//
// # Lifecycle Management
//
// The App handles sophisticated startup and shutdown orchestration:
//
//  1. Initialize observability (tracing, metrics)
//  2. Initialize bundles in registration order
//  3. Execute startup hooks
//  4. Start gRPC and HTTP servers
//  5. Start components in registration order
//  6. Mark service as ready
//
// Shutdown happens in reverse order with proper timeout handling and graceful termination.
package framework

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/datariot/forge/config"
	forgeHealth "github.com/datariot/forge/health"
)

// Component represents a service component that can be started and stopped.
// Components are the primary way to integrate your business logic with the Forge framework.
//
// Components are started in registration order during application startup and
// stopped in reverse order during graceful shutdown. Each component should
// handle its own resource management and cleanup.
//
// Example implementation:
//
//	type UserService struct {
//		db *sql.DB
//	}
//
//	func (s *UserService) Start(ctx context.Context) error {
//		// Initialize connections, start background workers, etc.
//		return s.db.PingContext(ctx)
//	}
//
//	func (s *UserService) Stop(ctx context.Context) error {
//		// Clean up resources, stop workers, close connections
//		return s.db.Close()
//	}
type Component interface {
	// Start initializes the component and starts any background processes.
	// This method is called during application startup after all bundles
	// have been initialized and servers have been started.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the component and cleans up resources.
	// This method is called during application shutdown with a timeout context.
	// Components should respect the context deadline for graceful termination.
	Stop(ctx context.Context) error
}

// HTTPRegistrar represents a component that can register HTTP routes.
type HTTPRegistrar interface {
	// RegisterHTTPRoutes registers HTTP routes with the provided mux.
	RegisterHTTPRoutes(mux *http.ServeMux)
}

// Registrar represents a service that can register gRPC handlers.
// Components that expose gRPC services should implement this interface
// to register their handlers with the framework's gRPC server.
//
// Example:
//
//	func (s *UserService) RegisterGRPC(server *grpc.Server) error {
//		userpb.RegisterUserServiceServer(server, s)
//		return nil
//	}
type Registrar interface {
	// RegisterGRPC registers gRPC service handlers with the provided server.
	// This method is called during application startup before the gRPC server starts.
	RegisterGRPC(server *grpc.Server) error
}

// HealthContributor represents a service that contributes health checks.
// Components implementing this interface can provide health checks that
// will be automatically registered with the health registry.
//
// Example:
//
//	func (s *UserService) HealthChecks() []health.Check {
//		return []forgeHealth.Check{
//			forgeHealth.NewBasicCheck(
//				forgeHealth.DefaultCheckConfig("database"),
//				func(ctx context.Context) error {
//					return s.db.PingContext(ctx)
//				},
//				func(ctx context.Context) error {
//					return s.db.PingContext(ctx)
//				},
//			),
//		}
//	}
type HealthContributor interface {
	// HealthChecks returns a slice of health checks that this component provides.
	// These checks will be automatically registered during application startup.
	HealthChecks() []forgeHealth.Check
}

// Bundle represents a reusable collection of functionality that can be added to an app.
// Bundles encapsulate common integrations like database connections, message queues,
// monitoring, etc. They are initialized in registration order during startup and
// stopped in reverse order during shutdown.
//
// Example bundle for PostgreSQL integration:
//
//	type PostgreSQLBundle struct {
//		config *DatabaseConfig
//		db     *sql.DB
//	}
//
//	func (b *PostgreSQLBundle) Name() string {
//		return "postgresql"
//	}
//
//	func (b *PostgreSQLBundle) Initialize(app *App) error {
//		db, err := sql.Open("postgres", b.config.URL)
//		if err != nil {
//			return err
//		}
//		b.db = db
//		return nil
//	}
//
//	func (b *PostgreSQLBundle) Stop(ctx context.Context) error {
//		if b.db != nil {
//			return b.db.Close()
//		}
//		return nil
//	}
type Bundle interface {
	// Name returns a unique identifier for this bundle.
	Name() string

	// Initialize sets up the bundle's functionality within the application.
	// This method is called during application startup before components are started.
	Initialize(app *App) error

	// Stop gracefully shuts down the bundle and cleans up resources.
	// This method is called during application shutdown in reverse initialization order.
	// Bundles should respect the context deadline for graceful termination.
	Stop(ctx context.Context) error
}

// StartupHook is called during application startup.
// Use startup hooks for custom initialization logic that needs to run
// after bundles are initialized but before the service is marked as ready.
type StartupHook func(ctx context.Context, app *App) error

// ShutdownHook is called during application shutdown.
// Use shutdown hooks for custom cleanup logic that needs to run
// before components are stopped. Hooks are executed in reverse registration order.
type ShutdownHook func(ctx context.Context, app *App) error

// App represents the main service application with lifecycle management.
// App orchestrates the entire application lifecycle including initialization,
// startup, runtime, and graceful shutdown of all registered components and services.
//
// The App follows a sophisticated startup sequence:
//  1. Configuration validation
//  2. Logging and observability initialization
//  3. Bundle initialization (in registration order)
//  4. Startup hook execution
//  5. gRPC and HTTP server startup
//  6. Component startup (in registration order)
//  7. Health check registration and readiness marking
//
// Shutdown happens in reverse order with proper timeout handling.
// The App ensures all resources are cleaned up gracefully on termination.
type App struct {
	config  *config.BaseConfig
	version string
	startAt time.Time

	// Core managers
	logging            *LoggingManager
	observability      *ObservabilityManager
	healthRegistry     *forgeHealth.Registry
	grpcHealthServer   *health.Server
	grpcHealthSyncStop chan struct{}

	// Servers
	grpcServer       *grpc.Server
	httpServer       *http.Server
	grpcListener     net.Listener
	httpServerConfig *HTTPServerConfig

	// Components and bundles
	components []Component
	registrars []Registrar
	bundles    []Bundle

	// gRPC interceptors
	unaryInterceptors  []grpc.UnaryServerInterceptor
	streamInterceptors []grpc.StreamServerInterceptor

	// Hooks
	startupHooks  []StartupHook
	shutdownHooks []ShutdownHook

	// Lifecycle
	mu       sync.RWMutex
	running  bool
	stopping bool
}

// AppOption configures the App during creation.
type AppOption func(*App) error

// New creates a new service application with the given options.
func New(options ...AppOption) (*App, error) {
	app := &App{
		version:            "development",
		startAt:            time.Now(),
		components:         make([]Component, 0),
		registrars:         make([]Registrar, 0),
		bundles:            make([]Bundle, 0),
		unaryInterceptors:  make([]grpc.UnaryServerInterceptor, 0),
		streamInterceptors: make([]grpc.StreamServerInterceptor, 0),
		startupHooks:       make([]StartupHook, 0),
		shutdownHooks:      make([]ShutdownHook, 0),
	}

	// Apply options
	for _, option := range options {
		if err := option(app); err != nil {
			return nil, fmt.Errorf("failed to apply app option: %w", err)
		}
	}

	// Validate required configuration
	if app.config == nil {
		return nil, fmt.Errorf("config is required")
	}

	if err := app.config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Initialize logging
	if app.logging == nil {
		app.logging = NewLoggingManager(app.config)
	}
	if err := app.logging.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize logging: %w", err)
	}

	// Initialize observability
	if app.observability == nil {
		obsConfig := NewObservabilityConfig(app.config, app.version)
		app.observability = NewObservabilityManager(obsConfig)
	}

	// Initialize health registry
	if app.healthRegistry == nil {
		healthLogger := NewHealthLogger(app.logging)
		app.healthRegistry = forgeHealth.NewRegistry(healthLogger)
	}

	return app, nil
}

// WithConfig sets the base configuration.
func WithConfig(config *config.BaseConfig) AppOption {
	return func(app *App) error {
		if config == nil {
			return fmt.Errorf("config cannot be nil")
		}
		app.config = config
		return nil
	}
}

// WithVersion sets the service version.
func WithVersion(version string) AppOption {
	return func(app *App) error {
		app.version = version
		return nil
	}
}

// WithComponent adds a component to be managed by the app.
func WithComponent(component Component) AppOption {
	return func(app *App) error {
		if component == nil {
			return fmt.Errorf("component cannot be nil")
		}
		app.components = append(app.components, component)
		return nil
	}
}

// WithGRPCRegistrar adds a gRPC service registrar.
func WithGRPCRegistrar(registrar Registrar) AppOption {
	return func(app *App) error {
		if registrar == nil {
			return fmt.Errorf("registrar cannot be nil")
		}
		app.registrars = append(app.registrars, registrar)
		return nil
	}
}

// WithBundle adds a bundle to the application.
func WithBundle(bundle Bundle) AppOption {
	return func(app *App) error {
		if bundle == nil {
			return fmt.Errorf("bundle cannot be nil")
		}
		app.bundles = append(app.bundles, bundle)
		return nil
	}
}

// WithHealthContributor adds health checks from a contributor.
func WithHealthContributor(contributor HealthContributor) AppOption {
	return func(app *App) error {
		if contributor == nil {
			return fmt.Errorf("contributor cannot be nil")
		}

		// This will be registered later during startup
		app.startupHooks = append(app.startupHooks, func(ctx context.Context, app *App) error {
			checks := contributor.HealthChecks()
			for _, check := range checks {
				config := forgeHealth.DefaultCheckConfig(check.Name())
				if err := app.healthRegistry.Register(check, config); err != nil {
					return fmt.Errorf("failed to register health check %s: %w", check.Name(), err)
				}
			}
			return nil
		})

		return nil
	}
}

// WithStartupHook adds a startup hook.
func WithStartupHook(hook StartupHook) AppOption {
	return func(app *App) error {
		if hook == nil {
			return fmt.Errorf("startup hook cannot be nil")
		}
		app.startupHooks = append(app.startupHooks, hook)
		return nil
	}
}

// WithShutdownHook adds a shutdown hook.
func WithShutdownHook(hook ShutdownHook) AppOption {
	return func(app *App) error {
		if hook == nil {
			return fmt.Errorf("shutdown hook cannot be nil")
		}
		app.shutdownHooks = append(app.shutdownHooks, hook)
		return nil
	}
}

// WithHTTPServerConfig sets a custom HTTP server configuration, overriding the
// framework's default endpoint, CORS, and request logging settings.
func WithHTTPServerConfig(cfg HTTPServerConfig) AppOption {
	return func(app *App) error {
		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("invalid HTTP server config: %w", err)
		}
		app.httpServerConfig = &cfg
		return nil
	}
}

// WithUnaryInterceptor adds a gRPC unary server interceptor.
func WithUnaryInterceptor(interceptor grpc.UnaryServerInterceptor) AppOption {
	return func(app *App) error {
		if interceptor == nil {
			return fmt.Errorf("unary interceptor cannot be nil")
		}
		app.unaryInterceptors = append(app.unaryInterceptors, interceptor)
		return nil
	}
}

// WithStreamInterceptor adds a gRPC stream server interceptor.
func WithStreamInterceptor(interceptor grpc.StreamServerInterceptor) AppOption {
	return func(app *App) error {
		if interceptor == nil {
			return fmt.Errorf("stream interceptor cannot be nil")
		}
		app.streamInterceptors = append(app.streamInterceptors, interceptor)
		return nil
	}
}

// Config returns the application configuration.
func (a *App) Config() *config.BaseConfig {
	return a.config
}

// Logger returns the logging manager.
func (a *App) Logger() *LoggingManager {
	return a.logging
}

// HealthRegistry returns the health registry.
func (a *App) HealthRegistry() *forgeHealth.Registry {
	return a.healthRegistry
}

// IsRunning returns true if the application is running.
func (a *App) IsRunning() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.running
}

// IsStopping returns true if the application is in the process of stopping.
func (a *App) IsStopping() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.stopping
}

// Uptime returns how long the service has been running.
func (a *App) Uptime() time.Duration {
	return time.Since(a.startAt)
}

// Run starts the application and blocks until shutdown.
func (a *App) Run(ctx context.Context) error {
	logger := a.logging.WithService(a.config.ServiceName, "run")

	logger.Info().
		Str("version", a.version).
		Str("grpc_addr", a.config.GRPCAddr).
		Str("http_addr", a.config.HTTPAddr).
		Msg("Starting service")

	// Start the application
	if err := a.Start(ctx); err != nil {
		return fmt.Errorf("failed to start application: %w", err)
	}

	// Wait for shutdown signal using standard library
	signalCtx, signalCancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer signalCancel()

	<-signalCtx.Done()
	if ctx.Err() != nil {
		logger.Info().Msg("Context cancelled")
	} else {
		logger.Info().Msg("Received shutdown signal")
	}

	// Shutdown the application
	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.config.ShutdownTimeout)
	defer cancel()

	if err := a.Stop(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("Error during shutdown")
		return err
	}

	logger.Info().Dur("uptime", a.Uptime()).Msg("Service stopped")
	return nil
}

// Start starts all application components.
func (a *App) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.running {
		return fmt.Errorf("application is already running")
	}

	logger := a.logging.WithService(a.config.ServiceName, "start")

	// Initialize observability
	if err := a.observability.Initialize(ctx); err != nil {
		return fmt.Errorf("failed to initialize observability: %w", err)
	}
	logger.Info().Msg("Observability initialized")

	// Initialize bundles
	for _, bundle := range a.bundles {
		bundleName := bundle.Name()
		if err := bundle.Initialize(a); err != nil {
			return fmt.Errorf("failed to initialize bundle %s: %w", bundleName, err)
		}
		logger.Info().Str("bundle", bundleName).Msg("Bundle initialized")
	}

	// Execute startup hooks
	for i, hook := range a.startupHooks {
		if err := hook(ctx, a); err != nil {
			return fmt.Errorf("startup hook %d failed: %w", i, err)
		}
		logger.Debug().Int("hook_index", i).Msg("Startup hook executed successfully")
	}

	// Start the health registry's background check runner now that bundles
	// and startup hooks have registered their checks, but before servers
	// start accepting traffic. From this point on, /health probes are
	// served from cached results instead of running checks live.
	//
	// This intentionally uses a background context rather than the Start
	// context above: the latter is only meant to bound startup itself (as
	// with startGRPCServer/startHTTPServer), not the runner's lifetime.
	// The runner is stopped explicitly in Stop.
	if err := a.healthRegistry.Start(context.Background()); err != nil {
		return fmt.Errorf("failed to start health registry: %w", err)
	}
	logger.Info().Msg("Health registry background runner started")

	// Create and start gRPC server only if registrars are configured
	if len(a.registrars) > 0 {
		if err := a.startGRPCServer(ctx); err != nil {
			return fmt.Errorf("failed to start gRPC server: %w", err)
		}
	}

	// Create and start HTTP server
	if err := a.startHTTPServer(ctx); err != nil {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	// Start components
	for i, component := range a.components {
		if err := component.Start(ctx); err != nil {
			return fmt.Errorf("failed to start component %d (%T): %w", i, component, err)
		}
	}

	// Mark service as ready after initial delay
	if a.config.ReadinessInitialDelay > 0 {
		go func() {
			time.Sleep(a.config.ReadinessInitialDelay)
			a.healthRegistry.SetReady(true)
			a.updateGRPCHealthStatus()
			logger.Info().
				Dur("delay", a.config.ReadinessInitialDelay).
				Msg("Service marked as ready")
		}()
	} else {
		a.healthRegistry.SetReady(true)
		a.updateGRPCHealthStatus()
		logger.Info().Msg("Service marked as ready")
	}

	// Keep the gRPC health service in sync with cached background check
	// results for the process lifetime, not just the startup snapshot.
	if a.grpcHealthServer != nil {
		a.grpcHealthSyncStop = make(chan struct{})
		go a.syncGRPCHealthLoop(a.grpcHealthSyncStop)
	}

	a.running = true
	logger.Info().Msg("Service started successfully")

	return nil
}

// Stop stops all application components gracefully using the shutdown orchestrator.
func (a *App) Stop(ctx context.Context) error {
	a.mu.Lock()

	if !a.running || a.stopping {
		a.mu.Unlock()
		return nil
	}

	a.stopping = true
	a.mu.Unlock()

	logger := a.logging.WithService(a.config.ServiceName, "stop")
	logger.Info().Msg("Stopping service")

	// Mark service as not ready immediately
	a.healthRegistry.SetReady(false)
	a.updateGRPCHealthStatus()

	if a.grpcHealthSyncStop != nil {
		close(a.grpcHealthSyncStop)
		a.grpcHealthSyncStop = nil
	}

	// Stop the health registry's background check runner. This waits for
	// in-flight check goroutines to notice cancellation and exit, bounded
	// by ctx, so shutdown doesn't hang on a wedged dependency check.
	if err := a.healthRegistry.Stop(ctx); err != nil {
		logger.Warn().Err(err).Msg("health registry runner did not stop cleanly")
	}

	// Create shutdown orchestrator with proper timeout
	orchestrator := NewShutdownOrchestrator(a.config.ShutdownTimeout)

	// Register shutdown hooks in reverse of the desired execution order.
	// The orchestrator executes hooks in reverse-registration order, so we
	// register: bundles → components → user-hooks, which executes as:
	// user-hooks → components → bundles (correct: business logic stops
	// before the infrastructure it depends on).

	// 1. Bundles (registered first, executed last — infrastructure tears down after consumers)
	for i := 0; i < len(a.bundles); i++ {
		bundle := a.bundles[i] // Create local copy for closure
		orchestrator.RegisterHook(fmt.Sprintf("bundle-%s", bundle.Name()), func(currentBundle Bundle) func(context.Context) error {
			return func(ctx context.Context) error {
				return currentBundle.Stop(ctx)
			}
		}(bundle))
	}

	// 2. Components (registered second, executed second — business logic stops before bundles)
	for i := 0; i < len(a.components); i++ {
		component := a.components[i] // Create local copy for closure
		orchestrator.RegisterHook(fmt.Sprintf("component-%d", i), func(currentComponent Component) func(context.Context) error {
			return func(ctx context.Context) error {
				return currentComponent.Stop(ctx)
			}
		}(component))
	}

	// 3. User-defined shutdown hooks (registered last, executed first)
	for i := 0; i < len(a.shutdownHooks); i++ {
		hook := a.shutdownHooks[i] // Create local copy for closure
		orchestrator.RegisterHook(fmt.Sprintf("user-hook-%d", i), func(currentHook ShutdownHook) func(context.Context) error {
			return func(ctx context.Context) error {
				return currentHook(ctx, a)
			}
		}(hook))
	}

	// 4. gRPC server
	if a.grpcServer != nil {
		orchestrator.RegisterHook("grpc-server", func(ctx context.Context) error {
			done := make(chan struct{})
			go func() {
				a.grpcServer.GracefulStop()
				close(done)
			}()

			select {
			case <-done:
				logger.Info().Msg("gRPC server stopped gracefully")
				return nil
			case <-ctx.Done():
				logger.Warn().Msg("gRPC graceful shutdown timeout, forcing stop")
				a.grpcServer.Stop()
				return fmt.Errorf("gRPC server forced stop due to timeout")
			}
		})
	}

	// 5. HTTP server
	if a.httpServer != nil {
		orchestrator.RegisterHook("http-server", func(ctx context.Context) error {
			if err := a.httpServer.Shutdown(ctx); err != nil {
				return fmt.Errorf("HTTP server shutdown error: %w", err)
			}
			logger.Info().Msg("HTTP server stopped")
			return nil
		})
	}

	// 6. Observability (last to maintain logging as long as possible)
	if a.observability != nil {
		orchestrator.RegisterHook("observability", func(ctx context.Context) error {
			if err := a.observability.Shutdown(ctx); err != nil {
				logger.Error().Err(err).Msg("Failed to shutdown observability")
				return err
			}
			return nil
		})
	}

	// Execute ordered shutdown
	shutdownErr := orchestrator.Shutdown(ctx)

	// Update app state
	a.mu.Lock()
	a.running = false
	a.stopping = false
	a.mu.Unlock()

	return shutdownErr
}

// startGRPCServer creates and starts the gRPC server.
func (a *App) startGRPCServer(ctx context.Context) error {
	// Create gRPC server options
	var opts []grpc.ServerOption

	// Add unary interceptors if any
	if len(a.unaryInterceptors) > 0 {
		opts = append(opts, grpc.ChainUnaryInterceptor(a.unaryInterceptors...))
	}

	// Add stream interceptors if any
	if len(a.streamInterceptors) > 0 {
		opts = append(opts, grpc.ChainStreamInterceptor(a.streamInterceptors...))
	}

	// Apply configured message size and connection timeout limits
	if a.config.GRPCMaxRecvMsgSize > 0 {
		opts = append(opts, grpc.MaxRecvMsgSize(a.config.GRPCMaxRecvMsgSize))
	}
	if a.config.GRPCMaxSendMsgSize > 0 {
		opts = append(opts, grpc.MaxSendMsgSize(a.config.GRPCMaxSendMsgSize))
	}
	if a.config.GRPCTimeout > 0 {
		opts = append(opts, grpc.ConnectionTimeout(a.config.GRPCTimeout))
	}

	// Create gRPC server with interceptors
	a.grpcServer = grpc.NewServer(opts...)

	// Initialize and register standard gRPC health service
	a.grpcHealthServer = health.NewServer()
	grpc_health_v1.RegisterHealthServer(a.grpcServer, a.grpcHealthServer)

	// Enable gRPC reflection if configured
	if a.config.ShouldEnableReflection() {
		reflection.Register(a.grpcServer)
	}

	// Register user services
	for i, registrar := range a.registrars {
		if err := registrar.RegisterGRPC(a.grpcServer); err != nil {
			return fmt.Errorf("failed to register gRPC service %d (%T): %w", i, registrar, err)
		}
	}

	// Create listener
	listener, err := net.Listen("tcp", a.config.GRPCAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", a.config.GRPCAddr, err)
	}
	a.grpcListener = listener

	// Start server in goroutine
	go func() {
		logger := a.logging.WithService(a.config.ServiceName, "grpc")
		logger.Info().Str("addr", a.config.GRPCAddr).Msg("Starting gRPC server")

		if err := a.grpcServer.Serve(listener); err != nil {
			logger.Error().Err(err).Msg("gRPC server error")
		}
	}()

	return nil
}

// startHTTPServer creates and starts the enhanced HTTP server.
func (a *App) startHTTPServer(ctx context.Context) error {
	// Use the user-supplied HTTP server configuration if provided, otherwise
	// fall back to defaults driven by the base config's toggles.
	var httpConfig HTTPServerConfig
	if a.httpServerConfig != nil {
		httpConfig = *a.httpServerConfig
	} else {
		httpConfig = DefaultHTTPServerConfig()
		httpConfig.EnableCORS = a.config.EnableCORS
		httpConfig.CORSOrigins = a.config.CORSOrigins
		httpConfig.EnableMetrics = a.config.EnableMetrics
		httpConfig.EnableRequestLogging = a.config.EnableRequestLogging
	}

	// Build enhanced HTTP server
	builder := NewHTTPServerBuilder(a, httpConfig)

	// Register custom routes if any components provide them
	for _, component := range a.components {
		if httpRegistrar, ok := component.(HTTPRegistrar); ok {
			builder.RegisterCustomRoutes(httpRegistrar.RegisterHTTPRoutes)
		}
	}

	httpServer, err := builder.Build()
	if err != nil {
		return fmt.Errorf("failed to build HTTP server: %w", err)
	}
	a.httpServer = httpServer

	// Create listener synchronously so bind failures (e.g. port already in use)
	// are reported to the caller instead of only being logged from the goroutine.
	listener, err := net.Listen("tcp", a.config.HTTPAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", a.config.HTTPAddr, err)
	}

	// Start server in goroutine
	go func() {
		logger := a.logging.WithService(a.config.ServiceName, "http")
		logger.Info().Str("addr", a.config.HTTPAddr).Msg("Starting HTTP server")

		if err := a.httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error().Err(err).Msg("HTTP server error")
		}
	}()

	return nil
}

// handleHealth handles the /health endpoint.
func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status := a.healthRegistry.CheckHealth(ctx)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status.HTTPStatus())

	if data, err := status.JSON(); err == nil {
		_, _ = w.Write(data)
	} else {
		_, _ = w.Write([]byte(`{"status":"error","message":"failed to serialize health status"}`))
	}
}

// handleReady handles the /health/ready endpoint.
func (a *App) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status := a.healthRegistry.CheckReadiness(ctx)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status.HTTPStatus())

	if data, err := status.JSON(); err == nil {
		_, _ = w.Write(data)
	} else {
		_, _ = w.Write([]byte(`{"status":"error","message":"failed to serialize readiness status"}`))
	}
}

// handleLive handles the /health/live endpoint.
func (a *App) handleLive(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status := a.healthRegistry.CheckLiveness(ctx)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status.HTTPStatus())

	if data, err := status.JSON(); err == nil {
		_, _ = w.Write(data)
	} else {
		_, _ = w.Write([]byte(`{"status":"error","message":"failed to serialize liveness status"}`))
	}
}

// updateGRPCHealthStatus synchronizes the Forge health registry status with the gRPC health service.
// This ensures that gRPC health checks reflect the same status as HTTP health endpoints.
func (a *App) updateGRPCHealthStatus() {
	if a.grpcHealthServer == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check overall readiness status
	readinessStatus := a.healthRegistry.CheckReadiness(ctx)

	// Update overall service status
	if readinessStatus.IsReady() {
		a.grpcHealthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	} else {
		a.grpcHealthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	}

	// Update individual service statuses based on individual health checks
	for checkName, checkResult := range readinessStatus.Details {
		if checkResult.IsHealthy() {
			a.grpcHealthServer.SetServingStatus(checkName, grpc_health_v1.HealthCheckResponse_SERVING)
		} else {
			a.grpcHealthServer.SetServingStatus(checkName, grpc_health_v1.HealthCheckResponse_NOT_SERVING)
		}
	}
}

// grpcHealthSyncInterval is how often the gRPC health service is refreshed
// from the health registry's cached check results.
const grpcHealthSyncInterval = 5 * time.Second

// syncGRPCHealthLoop periodically mirrors health registry status into the
// gRPC health service until the stop channel is closed. Reads are served
// from the registry's cache, so each tick is a lock-and-copy, not live checks.
func (a *App) syncGRPCHealthLoop(stop <-chan struct{}) {
	ticker := time.NewTicker(grpcHealthSyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			a.updateGRPCHealthStatus()
		}
	}
}
