// Package configloader provides automatic configuration loading and management for Forge applications.
//
// The configuration loader bundle provides:
//   - Automatic loading from multiple sources (files, environment, defaults)
//   - Support for YAML, JSON, and TOML configuration files
//   - Environment variable binding with struct tag support
//   - Configuration validation and merging with priority handling
//   - Hot reload and file watching capabilities
//   - Secure handling of sensitive configuration data
//   - Configuration source tracking and debugging
//
// # Basic Usage
//
// Define your configuration struct with appropriate tags:
//
//	type MyServiceConfig struct {
//		config.BaseConfig `yaml:",inline" env:",inline"`
//
//		// Service-specific configuration
//		DatabaseURL string `yaml:"database_url" env:"DATABASE_URL" validate:"required"`
//		APIKey      string `yaml:"api_key" env:"API_KEY" validate:"required" sensitive:"true"`
//		Debug       bool   `yaml:"debug" env:"DEBUG" default:"false"`
//	}
//
// Load configuration automatically:
//
//	loader := configloader.New(configloader.Config{
//		ConfigPaths: []string{"./config.yaml", "/etc/myservice/config.yaml"},
//		EnvPrefix:   "MYSERVICE",
//		WatchFiles:  true,
//	})
//
//	var cfg MyServiceConfig
//	if err := loader.Load(&cfg); err != nil {
//		log.Fatal(err)
//	}
//
// # Configuration Sources (Priority Order)
//
// 1. Command line flags (highest priority)
// 2. Environment variables
// 3. Configuration files (first found file wins)
// 4. Default values from struct tags (lowest priority)
//
// # File Formats Supported
//
//   - YAML: config.yaml, config.yml
//   - JSON: config.json
//   - TOML: config.toml (future)
//
// # Environment Variable Binding
//
// The loader automatically binds environment variables based on:
//
//   - Field names (converted to UPPER_SNAKE_CASE)
//
//   - `env` struct tags for custom names
//
//   - `envPrefix` configuration for namespacing
//
//     DatabaseURL string `env:"DATABASE_URL"`           // Exact name
//     APITimeout  int    `env:"API_TIMEOUT_SECONDS"`    // Custom name
//     Debug       bool   // Automatic: DEBUG or MYSERVICE_DEBUG (with prefix)
//
// # Hot Reload
//
// Enable automatic configuration reloading when files change:
//
//	loader.OnConfigChange(func(newConfig *MyServiceConfig) {
//		log.Printf("Configuration reloaded: %+v", newConfig)
//		// Handle configuration changes
//	})
//
// # Secure Configuration
//
// Mark sensitive fields to prevent logging:
//
//	APIKey string `yaml:"api_key" env:"API_KEY" sensitive:"true"`
//
// The loader will redact sensitive fields in logs and error messages.
package configloader

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
	"gopkg.in/yaml.v3"

	"github.com/datariot/forge/errors"
	"github.com/datariot/forge/framework"
)

// Config contains configuration loader settings.
type Config struct {
	// ConfigPaths is a list of configuration file paths to search.
	// The first existing file will be used.
	ConfigPaths []string

	// EnvPrefix is prepended to environment variable names.
	// For example, with prefix "MYSERVICE", field "Debug" becomes "MYSERVICE_DEBUG".
	EnvPrefix string

	// WatchFiles enables automatic reloading when configuration files change.
	WatchFiles bool

	// RequireConfigFile determines if a configuration file must exist.
	// If false, the loader will work with environment variables and defaults only.
	RequireConfigFile bool

	// ValidateOnLoad enables automatic validation using the Validator interface.
	ValidateOnLoad bool

	// SecureLogging prevents sensitive fields from appearing in logs.
	SecureLogging bool

	// Security configuration
	MaxFileSize      int64       // Maximum configuration file size (default: 1MB)
	AllowedPaths     []string    // Allowed configuration file directories
	RequiredFileMode os.FileMode // Required file permissions (default: 0o644)
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		ConfigPaths: []string{
			"./config.yaml",
			"./config.yml",
			"./config.json",
			"./config/config.yaml",
			"/etc/forge/config.yaml",
		},
		WatchFiles:        false, // Disabled by default for security
		RequireConfigFile: false,
		ValidateOnLoad:    true,
		SecureLogging:     true,
		MaxFileSize:       1024 * 1024, // 1MB
		RequiredFileMode:  0o644,       // rw-r--r--
	}
}

// Validate validates the configuration loader settings.
func (c *Config) Validate() error {
	if len(c.ConfigPaths) == 0 {
		return stderrors.New("at least one config path must be specified")
	}

	// Validate config paths
	for _, path := range c.ConfigPaths {
		if path == "" {
			return stderrors.New("config path cannot be empty")
		}
		if !filepath.IsAbs(path) && !strings.HasPrefix(path, "./") {
			return fmt.Errorf("config path must be absolute or relative (starting with ./): %s", path)
		}
	}

	return nil
}

// Bundle provides configuration loading for Forge applications.
type Bundle struct {
	config       Config
	loader       *Loader
	watcher      *fsnotify.Watcher
	watchedFiles []string
	logger       zerolog.Logger
	mu           sync.RWMutex
}

// NewBundle creates a new configuration loading bundle.
func NewBundle(config Config) *Bundle {
	return &Bundle{
		config: config,
	}
}

// Name returns the bundle name.
func (b *Bundle) Name() string {
	return "config-loader"
}

// Initialize sets up the configuration loader.
func (b *Bundle) Initialize(app *framework.App) error {
	if err := b.config.Validate(); err != nil {
		return errors.ErrInvalidConfiguration.WithMessage("Configuration loader validation failed").WithCause(err)
	}

	if app != nil {
		b.logger = app.Logger().WithService("config-loader", "configloader")
	} else {
		b.logger = zerolog.Nop()
	}

	// Create loader
	b.loader = &Loader{
		config: b.config,
	}

	// Initialize file watcher if enabled
	if b.config.WatchFiles {
		if err := b.initializeWatcher(); err != nil {
			return fmt.Errorf("failed to initialize file watcher: %w", err)
		}
	}

	return nil
}

// Loader returns the configuration loader instance.
func (b *Bundle) Loader() *Loader {
	return b.loader
}

// Stop implements the Bundle interface for graceful shutdown.
// Stops file watching and cleans up resources respecting the context deadline.
func (b *Bundle) Stop(ctx context.Context) error {
	if b.watcher == nil {
		return nil
	}

	// Channel to signal when watcher is closed
	done := make(chan error, 1)
	go func() {
		done <- b.watcher.Close()
	}()

	// Wait for either successful closure or context timeout
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		// Force close after timeout
		b.watcher.Close()
		return fmt.Errorf("config watcher close timed out: %w", ctx.Err())
	}
}

// Close is deprecated. Use Stop() instead for proper lifecycle integration.
// Maintained for backward compatibility.
func (b *Bundle) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return b.Stop(ctx)
}

// initializeWatcher sets up file system watching for hot reload.
func (b *Bundle) initializeWatcher() error {
	var err error
	b.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}

	// Add existing config files to watcher
	for _, path := range b.config.ConfigPaths {
		if _, err := os.Stat(path); err == nil {
			if err := b.watcher.Add(path); err != nil {
				// Log warning but don't fail
				continue
			}
			b.watchedFiles = append(b.watchedFiles, path)
		}
	}

	return nil
}

// Loader provides configuration loading functionality.
type Loader struct {
	config          Config
	loadedFrom      string
	changeCallbacks []func(interface{})
	mu              sync.RWMutex
}

// LoadResult contains information about the configuration loading process.
type LoadResult struct {
	LoadedFrom       string   `json:"loaded_from"`
	Sources          []string `json:"sources"`
	EnvVarsUsed      []string `json:"env_vars_used"`
	DefaultsApplied  []string `json:"defaults_applied"`
	ValidationErrors []string `json:"validation_errors,omitempty"`
}

// Load loads configuration into the provided struct from multiple sources.
func (l *Loader) Load(dest interface{}) (*LoadResult, error) {
	if dest == nil {
		return nil, stderrors.New("destination cannot be nil")
	}

	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.Elem().Kind() != reflect.Struct {
		return nil, stderrors.New("destination must be a pointer to a struct")
	}

	result := &LoadResult{
		Sources:         []string{},
		EnvVarsUsed:     []string{},
		DefaultsApplied: []string{},
	}

	// 1. Apply default values from struct tags
	if err := l.applyDefaults(destValue.Elem(), result); err != nil {
		return nil, fmt.Errorf("failed to apply defaults: %w", err)
	}

	// 2. Load from configuration file
	configFile, err := l.findConfigFile()
	if err != nil && l.config.RequireConfigFile {
		return nil, fmt.Errorf("required configuration file not found: %w", err)
	}

	if configFile != "" {
		if err := l.loadFromFile(configFile, dest); err != nil {
			return nil, fmt.Errorf("failed to load from file %s: %w", configFile, err)
		}
		result.LoadedFrom = configFile
		result.Sources = append(result.Sources, "file:"+configFile)
	}

	// 3. Apply environment variable overrides
	if err := l.applyEnvironmentVariables(destValue.Elem(), result); err != nil {
		return nil, fmt.Errorf("failed to apply environment variables: %w", err)
	}

	// 4. Validate configuration if enabled
	if l.config.ValidateOnLoad {
		if validator, ok := dest.(interface{ Validate() error }); ok {
			if err := validator.Validate(); err != nil {
				result.ValidationErrors = []string{err.Error()}
				return result, fmt.Errorf("configuration validation failed: %w", err)
			}
		}
	}

	l.mu.Lock()
	l.loadedFrom = result.LoadedFrom
	l.mu.Unlock()

	return result, nil
}

// findConfigFile finds the first existing configuration file.
func (l *Loader) findConfigFile() (string, error) {
	for _, path := range l.config.ConfigPaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no configuration file found in paths: %v", l.config.ConfigPaths)
}

// loadFromFile loads configuration from a file based on its extension with security validation.
func (l *Loader) loadFromFile(filename string, dest interface{}) error {
	// Validate file path for security
	if err := l.validateFilePath(filename); err != nil {
		return fmt.Errorf("invalid config file path: %w", err)
	}

	// Check file permissions and size
	info, err := os.Stat(filename)
	if err != nil {
		return fmt.Errorf("failed to stat config file: %w", err)
	}

	// Security: Check file permissions
	if l.config.RequiredFileMode != 0 && info.Mode().Perm() != l.config.RequiredFileMode {
		if info.Mode().Perm()&0o002 != 0 {
			return fmt.Errorf("configuration file %s is world-writable, security risk", filename)
		}
		if info.Mode().Perm()&0o020 != 0 {
			return fmt.Errorf("configuration file %s is group-writable, security risk", filename)
		}
	}

	// Security: Check file size to prevent DoS
	if l.config.MaxFileSize > 0 && info.Size() > l.config.MaxFileSize {
		return fmt.Errorf("configuration file %s is too large (%d bytes), maximum %d bytes allowed",
			filename, info.Size(), l.config.MaxFileSize)
	}

	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	// Use io.LimitReader to enforce size limits
	limitedReader := io.LimitReader(file, l.config.MaxFileSize)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	ext := filepath.Ext(filename)
	switch strings.ToLower(ext) {
	case ".yaml", ".yml":
		return l.loadYAML(data, dest)
	case ".json":
		return l.loadJSON(data, dest)
	default:
		return fmt.Errorf("unsupported configuration file format: %s", ext)
	}
}

// loadYAML loads configuration from YAML data.
func (l *Loader) loadYAML(data []byte, dest interface{}) error {
	if err := yaml.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	return nil
}

// loadJSON loads configuration from JSON data.
func (l *Loader) loadJSON(data []byte, dest interface{}) error {
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}
	return nil
}

// applyDefaults applies default values from struct tags.
func (l *Loader) applyDefaults(structValue reflect.Value, result *LoadResult) error {
	structType := structValue.Type()

	for i := 0; i < structValue.NumField(); i++ {
		field := structValue.Field(i)
		fieldType := structType.Field(i)

		// Skip unexported fields
		if !field.CanSet() {
			continue
		}

		// Handle embedded structs
		if fieldType.Anonymous && field.Kind() == reflect.Struct {
			if err := l.applyDefaults(field, result); err != nil {
				return err
			}
			continue
		}

		// Apply default value if specified
		if defaultValue := fieldType.Tag.Get("default"); defaultValue != "" {
			if err := l.setFieldValue(field, defaultValue); err != nil {
				return fmt.Errorf("failed to set default for field %s: %w", fieldType.Name, err)
			}
			result.DefaultsApplied = append(result.DefaultsApplied, fieldType.Name+"="+defaultValue)
		}
	}

	return nil
}

// applyEnvironmentVariables applies environment variable overrides.
func (l *Loader) applyEnvironmentVariables(structValue reflect.Value, result *LoadResult) error {
	structType := structValue.Type()

	for i := 0; i < structValue.NumField(); i++ {
		field := structValue.Field(i)
		fieldType := structType.Field(i)

		// Skip unexported fields
		if !field.CanSet() {
			continue
		}

		// Handle embedded structs
		if fieldType.Anonymous && field.Kind() == reflect.Struct {
			if err := l.applyEnvironmentVariables(field, result); err != nil {
				return err
			}
			continue
		}

		// Determine environment variable name
		envName := l.getEnvVarName(fieldType)
		if envName == "" {
			continue
		}

		// Check if environment variable exists
		if envValue, exists := os.LookupEnv(envName); exists {
			if err := l.setFieldValue(field, envValue); err != nil {
				return fmt.Errorf("failed to set field %s from env %s: %w", fieldType.Name, envName, err)
			}
			result.EnvVarsUsed = append(result.EnvVarsUsed, envName+"="+l.sanitizeValue(fieldType, envValue))
		}
	}

	return nil
}

// getEnvVarName determines the environment variable name for a field.
func (l *Loader) getEnvVarName(field reflect.StructField) string {
	// Check for explicit env tag
	if envName := field.Tag.Get("env"); envName != "" {
		if envName == "-" {
			return "" // Explicitly disabled
		}
		return envName
	}

	// Generate name from field name
	envName := toEnvVarName(field.Name)
	if l.config.EnvPrefix != "" {
		envName = l.config.EnvPrefix + "_" + envName
	}

	return envName
}

// setFieldValue sets a field value from a string representation.
func (l *Loader) setFieldValue(field reflect.Value, value string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Bool:
		if boolVal, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("invalid boolean value: %s", value)
		} else {
			field.SetBool(boolVal)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if field.Type() == reflect.TypeOf(time.Duration(0)) {
			// Handle time.Duration specially
			if duration, err := time.ParseDuration(value); err != nil {
				return fmt.Errorf("invalid duration value: %s", value)
			} else {
				field.SetInt(int64(duration))
			}
		} else {
			if intVal, err := strconv.ParseInt(value, 10, 64); err != nil {
				return fmt.Errorf("invalid integer value: %s", value)
			} else {
				field.SetInt(intVal)
			}
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if uintVal, err := strconv.ParseUint(value, 10, 64); err != nil {
			return fmt.Errorf("invalid unsigned integer value: %s", value)
		} else {
			field.SetUint(uintVal)
		}
	case reflect.Float32, reflect.Float64:
		if floatVal, err := strconv.ParseFloat(value, 64); err != nil {
			return fmt.Errorf("invalid float value: %s", value)
		} else {
			field.SetFloat(floatVal)
		}
	case reflect.Slice:
		// Handle string slices (comma-separated values)
		if field.Type().Elem().Kind() == reflect.String {
			if value != "" {
				values := strings.Split(value, ",")
				for i, v := range values {
					values[i] = strings.TrimSpace(v)
				}
				field.Set(reflect.ValueOf(values))
			}
		} else {
			return stderrors.New("unsupported slice type for field")
		}
	default:
		return fmt.Errorf("unsupported field type: %s", field.Kind())
	}

	return nil
}

// sanitizeValue sanitizes configuration values for logging with comprehensive sensitive data detection.
func (l *Loader) sanitizeValue(field reflect.StructField, value string) string {
	if !l.config.SecureLogging {
		return value
	}

	// Check if field is explicitly marked as sensitive
	if field.Tag.Get("sensitive") == "true" {
		return "[REDACTED]"
	}

	// Check env tag for sensitive keywords
	if envTag := field.Tag.Get("env"); envTag != "" {
		envLower := strings.ToLower(envTag)
		if l.containsSensitiveKeyword(envLower) {
			return "[REDACTED]"
		}
	}

	// Check field name for sensitive keywords
	fieldName := strings.ToLower(field.Name)
	if l.containsSensitiveKeyword(fieldName) {
		return "[REDACTED]"
	}

	// Check if value looks like sensitive data (heuristic detection)
	if l.looksLikeSensitiveData(value) {
		return "[REDACTED-DETECTED]"
	}

	return value
}

// containsSensitiveKeyword checks if a string contains sensitive keywords.
func (l *Loader) containsSensitiveKeyword(text string) bool {
	sensitiveKeywords := []string{
		"password", "secret", "key", "token", "credential",
		"auth", "cert", "private", "jwt", "oauth", "api_key",
		"access_key", "secret_key", "private_key", "client_secret",
		"session", "cookie", "tls", "ssl", "pgp", "gpg",
	}

	for _, keyword := range sensitiveKeywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

// looksLikeSensitiveData uses heuristics to detect potential sensitive data.
func (l *Loader) looksLikeSensitiveData(value string) bool {
	if len(value) == 0 {
		return false
	}

	// Base64-encoded data (likely sensitive)
	if len(value) > 20 && len(value)%4 == 0 {
		// Check if it's base64
		if strings.TrimRight(value, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/=") == "" {
			return true
		}
	}

	// Hex-encoded data (32+ characters, likely a hash or key)
	if len(value) >= 32 && len(value) <= 128 {
		if strings.TrimRight(value, "ABCDEFabcdef0123456789") == "" {
			return true
		}
	}

	// JWT tokens (three base64 parts separated by dots)
	if strings.Count(value, ".") == 2 {
		parts := strings.Split(value, ".")
		if len(parts) == 3 && len(parts[0]) > 10 && len(parts[1]) > 10 && len(parts[2]) > 10 {
			return true
		}
	}

	// Very long alphanumeric strings (likely API keys)
	if len(value) > 40 && strings.TrimRight(value, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_") == "" {
		return true
	}

	return false
}

// toEnvVarName converts a field name to environment variable naming convention.
func toEnvVarName(fieldName string) string {
	var result strings.Builder
	for i, char := range fieldName {
		if i > 0 && char >= 'A' && char <= 'Z' {
			result.WriteByte('_')
		}
		result.WriteRune(char)
	}
	return strings.ToUpper(result.String())
}

// OnConfigChange registers a callback for configuration changes (hot reload).
func (l *Loader) OnConfigChange(callback func(interface{})) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.changeCallbacks = append(l.changeCallbacks, callback)
}

// StartWatching starts watching configuration files for changes.
func (b *Bundle) StartWatching(ctx context.Context, dest interface{}) error {
	if b.watcher == nil {
		return stderrors.New("file watcher not initialized")
	}

	go func() {
		for {
			select {
			case event, ok := <-b.watcher.Events:
				if !ok {
					return
				}

				// Handle file modification events
				if event.Op&fsnotify.Write == fsnotify.Write {
					// Reload configuration
					if _, err := b.loader.Load(dest); err != nil {
						b.logger.Error().Err(err).Msg("configuration reload error")
						continue
					}

					// Notify callbacks
					b.loader.mu.RLock()
					callbacks := make([]func(interface{}), len(b.loader.changeCallbacks))
					copy(callbacks, b.loader.changeCallbacks)
					b.loader.mu.RUnlock()

					for _, callback := range callbacks {
						callback(dest)
					}
				}

			case err, ok := <-b.watcher.Errors:
				if !ok {
					return
				}
				b.logger.Error().Err(err).Msg("configuration watcher error")

			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// GetConfigInfo returns information about the loaded configuration.
func (l *Loader) GetConfigInfo() map[string]interface{} {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return map[string]interface{}{
		"loaded_from":      l.loadedFrom,
		"config_paths":     l.config.ConfigPaths,
		"env_prefix":       l.config.EnvPrefix,
		"watch_enabled":    l.config.WatchFiles,
		"require_file":     l.config.RequireConfigFile,
		"validate_on_load": l.config.ValidateOnLoad,
		"secure_logging":   l.config.SecureLogging,
	}
}

// LoadFromString loads configuration from a string (useful for testing).
func (l *Loader) LoadFromString(data, format string, dest interface{}) error {
	switch strings.ToLower(format) {
	case "yaml", "yml":
		return l.loadYAML([]byte(data), dest)
	case "json":
		return l.loadJSON([]byte(data), dest)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

// MustLoad loads configuration and panics on error (useful for application startup).
func (l *Loader) MustLoad(dest interface{}) *LoadResult {
	result, err := l.Load(dest)
	if err != nil {
		panic(fmt.Sprintf("Configuration loading failed: %v", err))
	}
	return result
}

// Reload reloads configuration from all sources.
func (l *Loader) Reload(dest interface{}) (*LoadResult, error) {
	return l.Load(dest) // Load method already handles all sources
}

// validateFilePath validates that a configuration file path is safe to access.
// Relative paths are resolved to absolute paths via filepath.Abs before validation.
// Paths containing ".." traversal sequences are rejected regardless of resolution outcome.
func (l *Loader) validateFilePath(filename string) error {
	if filename == "" {
		return stderrors.New("configuration file path cannot be empty")
	}

	// Reject inputs that contain path traversal sequences before any resolution.
	// filepath.Clean / filepath.Abs would silently resolve "../../../etc/passwd" to
	// a valid absolute path, making it look legitimate. We block such inputs explicitly.
	if strings.Contains(filepath.Clean(filename), "..") {
		return stderrors.New("path traversal not allowed in configuration file path")
	}

	// Resolve relative paths to absolute
	absPath, err := filepath.Abs(filename)
	if err != nil {
		return fmt.Errorf("failed to resolve configuration file path: %w", err)
	}

	cleanPath := filepath.Clean(absPath)

	// If allowed paths are configured, validate against them
	if len(l.config.AllowedPaths) > 0 {
		allowed := false
		for _, allowedPath := range l.config.AllowedPaths {
			cleanAllowed := filepath.Clean(allowedPath)
			if strings.HasPrefix(cleanPath, cleanAllowed) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("configuration file path %s not in allowed paths: %v", cleanPath, l.config.AllowedPaths)
		}
	}

	return nil
}

// Helper functions for configuration loading

// LoadConfig is a convenience function for loading configuration with default settings.
func LoadConfig[T any](configPaths ...string) (*T, *LoadResult, error) {
	config := DefaultConfig()
	if len(configPaths) > 0 {
		config.ConfigPaths = configPaths
	}

	loader := &Loader{config: config}

	var cfg T
	result, err := loader.Load(&cfg)
	if err != nil {
		return nil, result, err
	}

	return &cfg, result, nil
}

// MustLoadConfig is a convenience function that panics on configuration loading errors.
func MustLoadConfig[T any](configPaths ...string) (*T, *LoadResult) {
	cfg, result, err := LoadConfig[T](configPaths...)
	if err != nil {
		panic(fmt.Sprintf("Configuration loading failed: %v", err))
	}
	return cfg, result
}
