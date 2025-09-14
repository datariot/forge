package framework

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/datariot/forge/config"
	"github.com/datariot/forge/health"
)

// Component represents a service component that can be started and stopped.
type Component interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// Registrar represents a service that can register gRPC handlers.
type Registrar interface {
	RegisterGRPC(server *grpc.Server) error
}

// HealthContributor represents a service that contributes health checks.
type HealthContributor interface {
	HealthChecks() []health.Check
}

// Bundle represents a reusable collection of functionality that can be added to an app.
type Bundle interface {
	Name() string
	Initialize(app *App) error
}

// StartupHook is called during application startup.
type StartupHook func(ctx context.Context, app *App) error

// ShutdownHook is called during application shutdown.
type ShutdownHook func(ctx context.Context, app *App) error

// App represents the main service application with lifecycle management.
type App struct {
	config  *config.BaseConfig
	version string
	startAt time.Time

	// Core managers
	logging        *LoggingManager
	observability  *ObservabilityManager
	healthRegistry *health.Registry

	// Servers
	grpcServer   *grpc.Server
	httpServer   *http.Server
	grpcListener net.Listener

	// Components and bundles
	components []Component
	registrars []Registrar
	bundles    []Bundle

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
		version:       "development",
		startAt:       time.Now(),
		components:    make([]Component, 0),
		registrars:    make([]Registrar, 0),
		bundles:       make([]Bundle, 0),
		startupHooks:  make([]StartupHook, 0),
		shutdownHooks: make([]ShutdownHook, 0),
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
		app.healthRegistry = health.NewRegistry(healthLogger)
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
				config := health.DefaultCheckConfig(check.Name())
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

// Config returns the application configuration.
func (a *App) Config() *config.BaseConfig {
	return a.config
}

// Logger returns the logging manager.
func (a *App) Logger() *LoggingManager {
	return a.logging
}

// HealthRegistry returns the health registry.
func (a *App) HealthRegistry() *health.Registry {
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

	// Wait for shutdown signal using signal context
	signalCtx, signalCancel := SignalContext(ctx)
	defer signalCancel()

	select {
	case <-signalCtx.Done():
		if ctx.Err() != nil {
			logger.Info().Msg("Context cancelled")
		} else {
			logger.Info().Msg("Received shutdown signal")
		}
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
	for i, bundle := range a.bundles {
		bundleName := bundle.Name()
		if err := bundle.Initialize(a); err != nil {
			return fmt.Errorf("failed to initialize bundle %d (%s): %w", i, bundleName, err)
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

	// Create and start gRPC server
	if err := a.startGRPCServer(ctx); err != nil {
		return fmt.Errorf("failed to start gRPC server: %w", err)
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
			logger.Info().
				Dur("delay", a.config.ReadinessInitialDelay).
				Msg("Service marked as ready")
		}()
	} else {
		a.healthRegistry.SetReady(true)
		logger.Info().Msg("Service marked as ready")
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

	// Create shutdown orchestrator with proper timeout
	orchestrator := NewShutdownOrchestrator(a.config.ShutdownTimeout)

	// Register shutdown hooks in proper order (reverse of startup)

	// 1. User-defined shutdown hooks (executed first, in reverse order)
	for i := len(a.shutdownHooks) - 1; i >= 0; i-- {
		hook := a.shutdownHooks[i] // Create local copy for closure
		orchestrator.RegisterHook(fmt.Sprintf("user-hook-%d", i), func(currentHook ShutdownHook) func(context.Context) error {
			return func(ctx context.Context) error {
				return currentHook(ctx, a)
			}
		}(hook))
	}

	// 2. Components (in reverse order of startup)
	for i := len(a.components) - 1; i >= 0; i-- {
		component := a.components[i] // Create local copy for closure
		orchestrator.RegisterHook(fmt.Sprintf("component-%d", i), func(currentComponent Component) func(context.Context) error {
			return func(ctx context.Context) error {
				return currentComponent.Stop(ctx)
			}
		}(component))
	}

	// 3. gRPC server
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

	// 4. HTTP server
	if a.httpServer != nil {
		orchestrator.RegisterHook("http-server", func(ctx context.Context) error {
			if err := a.httpServer.Shutdown(ctx); err != nil {
				return fmt.Errorf("HTTP server shutdown error: %w", err)
			}
			logger.Info().Msg("HTTP server stopped")
			return nil
		})
	}

	// 5. Observability (last to maintain logging as long as possible)
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
	// Create gRPC server (will implement server builder later)
	a.grpcServer = grpc.NewServer()

	// Register services
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

// startHTTPServer creates and starts the HTTP health server.
func (a *App) startHTTPServer(ctx context.Context) error {
	// Create HTTP server (will implement proper health server later)
	mux := http.NewServeMux()

	// Basic health endpoint
	mux.HandleFunc("/health", a.handleHealth)
	mux.HandleFunc("/health/ready", a.handleReady)
	mux.HandleFunc("/health/live", a.handleLive)

	a.httpServer = &http.Server{
		Addr:         a.config.HTTPAddr,
		Handler:      mux,
		ReadTimeout:  a.config.HTTPReadTimeout,
		WriteTimeout: a.config.HTTPWriteTimeout,
		IdleTimeout:  a.config.HTTPIdleTimeout,
	}

	// Start server in goroutine
	go func() {
		logger := a.logging.WithService(a.config.ServiceName, "http")
		logger.Info().Str("addr", a.config.HTTPAddr).Msg("Starting HTTP server")

		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
		w.Write(data)
	} else {
		w.Write([]byte(`{"status":"error","message":"failed to serialize health status"}`))
	}
}

// handleReady handles the /health/ready endpoint.
func (a *App) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status := a.healthRegistry.CheckReadiness(ctx)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status.HTTPStatus())

	if data, err := status.JSON(); err == nil {
		w.Write(data)
	} else {
		w.Write([]byte(`{"status":"error","message":"failed to serialize readiness status"}`))
	}
}

// handleLive handles the /health/live endpoint.
func (a *App) handleLive(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status := a.healthRegistry.CheckLiveness(ctx)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status.HTTPStatus())

	if data, err := status.JSON(); err == nil {
		w.Write(data)
	} else {
		w.Write([]byte(`{"status":"error","message":"failed to serialize liveness status"}`))
	}
}