package framework

import (
	"fmt"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/datariot/forge/config"
)

// LoggingManager manages logging configuration and provides logger instances.
type LoggingManager struct {
	config *config.BaseConfig
	logger zerolog.Logger
}

// NewLoggingManager creates a new logging manager with the given configuration.
func NewLoggingManager(config *config.BaseConfig) *LoggingManager {
	return &LoggingManager{
		config: config,
	}
}

// Initialize sets up the logging configuration.
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

// Logger returns the base logger instance.
func (lm *LoggingManager) Logger() *zerolog.Logger {
	return &lm.logger
}

// WithService creates a logger with service and component context.
func (lm *LoggingManager) WithService(service, component string) zerolog.Logger {
	return lm.logger.With().
		Str("service", service).
		Str("component", component).
		Logger()
}

// WithContext creates a logger with additional context fields.
func (lm *LoggingManager) WithContext(fields map[string]interface{}) zerolog.Logger {
	ctx := lm.logger.With()
	for key, value := range fields {
		ctx = ctx.Interface(key, value)
	}
	return ctx.Logger()
}

// NewHealthLogger creates a logger adapter for the health system.
func NewHealthLogger(logging *LoggingManager) *healthLoggerAdapter {
	return &healthLoggerAdapter{
		logger: logging.WithService(logging.config.ServiceName, "health"),
	}
}

type healthLoggerAdapter struct {
	logger zerolog.Logger
}

func (h *healthLoggerAdapter) Debug(msg string, fields ...interface{}) {
	event := h.logger.Debug()
	h.addFields(event, fields)
	event.Msg(msg)
}

func (h *healthLoggerAdapter) Info(msg string, fields ...interface{}) {
	event := h.logger.Info()
	h.addFields(event, fields)
	event.Msg(msg)
}

func (h *healthLoggerAdapter) Warn(msg string, fields ...interface{}) {
	event := h.logger.Warn()
	h.addFields(event, fields)
	event.Msg(msg)
}

func (h *healthLoggerAdapter) Error(msg string, fields ...interface{}) {
	event := h.logger.Error()
	h.addFields(event, fields)
	event.Msg(msg)
}

// addFields safely adds key-value pairs to a zerolog event
func (h *healthLoggerAdapter) addFields(event *zerolog.Event, fields []interface{}) {
	for i := 0; i < len(fields)-1; i += 2 {
		key, ok := fields[i].(string)
		if !ok {
			// If key is not a string, create a generic key
			key = fmt.Sprintf("field_%d", i/2)
		}
		event.Interface(key, fields[i+1])
	}
}
