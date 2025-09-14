package framework

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"

	"github.com/datariot/forge/config"
)

// ObservabilityConfig contains configuration for observability (tracing, metrics).
type ObservabilityConfig struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	OTELEndpoint   string
	SampleRate     float64
}

// NewObservabilityConfig creates a new observability config from base config.
func NewObservabilityConfig(baseConfig *config.BaseConfig, version string) *ObservabilityConfig {
	return &ObservabilityConfig{
		ServiceName:    baseConfig.ServiceName,
		ServiceVersion: version,
		Environment:    baseConfig.AppEnv,
		OTELEndpoint:   baseConfig.OTELEndpoint,
		SampleRate:     baseConfig.OTELSampleRate,
	}
}

// ObservabilityManager manages OpenTelemetry tracing and metrics.
type ObservabilityManager struct {
	config         *ObservabilityConfig
	traceProvider  *trace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	initialized    bool
}

// NewObservabilityManager creates a new observability manager.
func NewObservabilityManager(config *ObservabilityConfig) *ObservabilityManager {
	return &ObservabilityManager{
		config: config,
	}
}

// Initialize sets up OpenTelemetry tracing and metrics.
func (om *ObservabilityManager) Initialize(ctx context.Context) error {
	if om.initialized {
		return nil
	}

	// Set up tracing
	if err := om.initializeTracing(ctx); err != nil {
		return fmt.Errorf("failed to initialize tracing: %w", err)
	}

	// Set up metrics
	if err := om.initializeMetrics(ctx); err != nil {
		return fmt.Errorf("failed to initialize metrics: %w", err)
	}

	om.initialized = true
	return nil
}

// initializeTracing sets up OpenTelemetry tracing.
func (om *ObservabilityManager) initializeTracing(ctx context.Context) error {
	// Create OTLP trace exporter with conditional security
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(om.config.OTELEndpoint),
	}

	// Use insecure connection only for development environment
	if om.config.Environment == "development" {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	traceExporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// Create resource with service information
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(om.config.ServiceName),
			semconv.ServiceVersion(om.config.ServiceVersion),
			semconv.DeploymentEnvironment(om.config.Environment),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create resource: %w", err)
	}

	// Create trace provider with sampling
	om.traceProvider = trace.NewTracerProvider(
		trace.WithBatcher(traceExporter),
		trace.WithResource(res),
		trace.WithSampler(trace.TraceIDRatioBased(om.config.SampleRate)),
	)

	// Set as global trace provider
	otel.SetTracerProvider(om.traceProvider)

	// Set global propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return nil
}

// initializeMetrics sets up OpenTelemetry metrics.
func (om *ObservabilityManager) initializeMetrics(ctx context.Context) error {
	// Create resource with service information (reuse from tracing)
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(om.config.ServiceName),
			semconv.ServiceVersion(om.config.ServiceVersion),
			semconv.DeploymentEnvironment(om.config.Environment),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create resource for metrics: %w", err)
	}

	// Create meter provider with OTLP exporter
	// For now, we'll use the periodic reader with stdout exporter for development
	// In production, you would configure OTLP metrics exporter here
	om.meterProvider = sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		// Add readers/exporters here as needed
	)

	// Set as global meter provider
	otel.SetMeterProvider(om.meterProvider)

	return nil
}

// Shutdown gracefully shuts down the observability components.
func (om *ObservabilityManager) Shutdown(ctx context.Context) error {
	if !om.initialized {
		return nil
	}

	var shutdownErrors []error

	// Shutdown trace provider
	if om.traceProvider != nil {
		if err := om.traceProvider.Shutdown(ctx); err != nil {
			shutdownErrors = append(shutdownErrors, fmt.Errorf("failed to shutdown trace provider: %w", err))
		}
	}

	// Shutdown meter provider
	if om.meterProvider != nil {
		if err := om.meterProvider.Shutdown(ctx); err != nil {
			shutdownErrors = append(shutdownErrors, fmt.Errorf("failed to shutdown meter provider: %w", err))
		}
	}

	// Return first error if any occurred
	if len(shutdownErrors) > 0 {
		return shutdownErrors[0]
	}

	return nil
}

// GetTracer returns a tracer for the given name.
func (om *ObservabilityManager) GetTracer(name string) oteltrace.Tracer {
	if om.traceProvider == nil {
		return otel.Tracer(name)
	}
	return om.traceProvider.Tracer(name)
}

// GetMeter returns a meter for the given name.
func (om *ObservabilityManager) GetMeter(name string) otelmetric.Meter {
	if om.meterProvider == nil {
		return otel.Meter(name)
	}
	return om.meterProvider.Meter(name)
}