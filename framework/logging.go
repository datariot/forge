package framework

import (
	"fmt"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/datariot/forge/config"
)

// LoggingManager owns the application's zerolog configuration and hands out
// logger instances tagged with service and component context. Call
// Initialize before using Logger, WithService, or WithContext so the
// underlying logger has its output format (pretty console in development,
// JSON in production) and log level configured.
type LoggingManager struct {
	config *config.BaseConfig
	logger zerolog.Logger
}

// NewLoggingManager creates a LoggingManager bound to the given
// configuration. The returned manager's logger is not yet configured; call
// Initialize before using it.
func NewLoggingManager(config *config.BaseConfig) *LoggingManager {
	return &LoggingManager{
		config: config,
	}
}

// Initialize configures the manager's base logger from its BaseConfig: it
// sets the global zerolog level (falling back to Info if LogLevel doesn't
// parse), switches between a human-readable console writer in development
// and JSON output otherwise, and tags every log line with the service name
// and environment. The App calls this automatically during New, before
// observability and bundles are initialized.
func (lm *LoggingManager) Initialize() error {
	// Set global log level based on configuration
	level, err := zerolog.ParseLevel(lm.config.LogLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	// Configure output format
	if lm.config.IsDevelopment() {
		// Pretty console output for development
		lm.logger = log.Output(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "15:04:05",
		})
	} else {
		// JSON output for production
		lm.logger = log.Logger
	}

	// Add service context
	lm.logger = lm.logger.With().
		Str("service", lm.config.ServiceName).
		Str("env", lm.config.AppEnv).
		Logger()

	return nil
}

// Logger returns a pointer to the manager's base zerolog.Logger, already
// tagged with service and environment fields by Initialize.
func (lm *LoggingManager) Logger() *zerolog.Logger {
	return &lm.logger
}

// WithService returns a copy of the base logger with additional "service"
// and "component" fields attached, for use by a specific subsystem (e.g. the
// gRPC or HTTP server) so its log lines can be filtered independently.
func (lm *LoggingManager) WithService(service, component string) zerolog.Logger {
	return lm.logger.With().
		Str("service", service).
		Str("component", component).
		Logger()
}

// WithContext returns a copy of the base logger with an arbitrary set of
// key-value fields attached, one call to Interface per map entry, for
// callers that need structured fields beyond the fixed service/component
// pair provided by WithService.
func (lm *LoggingManager) WithContext(fields map[string]any) zerolog.Logger {
	ctx := lm.logger.With()
	for key, value := range fields {
		ctx = ctx.Interface(key, value)
	}
	return ctx.Logger()
}

// NewHealthLogger wraps a LoggingManager in an adapter satisfying the
// health package's logger interface, so the health registry's background
// checks log through the same structured, service-tagged zerolog output as
// the rest of the application instead of a separate logging path.
func NewHealthLogger(logging *LoggingManager) *healthLoggerAdapter {
	return &healthLoggerAdapter{
		logger: logging.WithService(logging.config.ServiceName, "health"),
	}
}

type healthLoggerAdapter struct {
	logger zerolog.Logger
}

func (h *healthLoggerAdapter) Debug(msg string, fields ...any) {
	event := h.logger.Debug()
	h.addFields(event, fields)
	event.Msg(msg)
}

func (h *healthLoggerAdapter) Info(msg string, fields ...any) {
	event := h.logger.Info()
	h.addFields(event, fields)
	event.Msg(msg)
}

func (h *healthLoggerAdapter) Warn(msg string, fields ...any) {
	event := h.logger.Warn()
	h.addFields(event, fields)
	event.Msg(msg)
}

func (h *healthLoggerAdapter) Error(msg string, fields ...any) {
	event := h.logger.Error()
	h.addFields(event, fields)
	event.Msg(msg)
}

// addFields safely adds key-value pairs to a zerolog event
func (h *healthLoggerAdapter) addFields(event *zerolog.Event, fields []any) {
	for i := 0; i < len(fields)-1; i += 2 {
		key, ok := fields[i].(string)
		if !ok {
			// If key is not a string, create a generic key
			key = fmt.Sprintf("field_%d", i/2)
		}
		event.Interface(key, fields[i+1])
	}
}
