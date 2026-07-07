// Package httpclient provides an HTTP client bundle for reliable service-to-service communication.
//
// The HTTP client bundle provides:
//   - HTTP client with circuit breaker protection
//   - Retry logic with exponential backoff
//   - Request/response logging and metrics
//   - JWT authentication integration
//   - Connection pooling and timeout management
//   - Service discovery integration (future)
//   - Request tracing and observability
//
// # Basic Usage
//
// Add the HTTP client bundle to your application:
//
//	config := httpclient.Config{
//		BaseURL: "https://api.example.com",
//		Timeout: 30 * time.Second,
//		RetryConfig: httpclient.RetryConfig{
//			MaxRetries: 3,
//			InitialInterval: 100 * time.Millisecond,
//			MaxInterval: 5 * time.Second,
//		},
//	}
//
//	bundle := httpclient.NewBundle(config)
//
//	app, err := framework.New(
//		framework.WithConfig(&baseConfig),
//		framework.WithBundle(bundle),
//	)
//
// # Making HTTP Calls
//
// The bundle provides a high-level client interface:
//
//	client := bundle.Client()
//
//	// GET request with automatic retries and circuit breaker
//	var user User
//	err := client.Get(ctx, "/users/123", &user)
//
//	// POST request with request body
//	createReq := CreateUserRequest{Name: "John", Email: "john@example.com"}
//	var createResp CreateUserResponse
//	err := client.Post(ctx, "/users", createReq, &createResp)
//
// # Authentication Integration
//
// The bundle automatically integrates with JWT authentication:
//
//	// JWT tokens are automatically injected into requests
//	err := client.Get(ctx, "/protected/resource", &response)
//
// # Circuit Breaker Protection
//
// The bundle provides automatic circuit breaker protection:
//
//	// Circuit breaker automatically opens on failures
//	// and closes when service recovers
//	err := client.Get(ctx, "/unreliable-service", &response)
//	if errors.Is(err, httpclient.ErrCircuitBreakerOpen) {
//		// Handle circuit breaker open state
//	}
package httpclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/rs/zerolog"
	"github.com/sony/gobreaker"

	"github.com/datariot/forge/errors"
	"github.com/datariot/forge/framework"
)

// CredentialProvider provides secure access to authentication credentials.
type CredentialProvider interface {
	// GetAPIKey returns the API key for authentication.
	GetAPIKey(ctx context.Context) (string, error)

	// GetJWTToken returns a JWT token for authentication.
	GetJWTToken(ctx context.Context) (string, error)
}

// StaticCredentialProvider provides static credentials (for development/testing only).
type StaticCredentialProvider struct {
	apiKey   string
	jwtToken string
}

// NewStaticCredentialProvider creates a credential provider with static values.
// WARNING: Only use for development/testing. In production, use secure credential stores.
func NewStaticCredentialProvider(apiKey, jwtToken string) *StaticCredentialProvider {
	return &StaticCredentialProvider{
		apiKey:   apiKey,
		jwtToken: jwtToken,
	}
}

// GetAPIKey returns the static API key.
func (p *StaticCredentialProvider) GetAPIKey(ctx context.Context) (string, error) {
	if p.apiKey == "" {
		return "", stderrors.New("no API key configured")
	}
	return p.apiKey, nil
}

// GetJWTToken returns the static JWT token.
func (p *StaticCredentialProvider) GetJWTToken(ctx context.Context) (string, error) {
	if p.jwtToken == "" {
		return "", stderrors.New("no JWT token configured")
	}
	return p.jwtToken, nil
}

// Config contains HTTP client configuration options.
type Config struct {
	// BaseURL is the base URL for all requests (optional).
	BaseURL string

	// Timeout is the overall request timeout.
	Timeout time.Duration

	// Transport configuration
	MaxIdleConns          int           // Maximum idle connections (default: 100)
	MaxIdleConnsPerHost   int           // Maximum idle connections per host (default: 10)
	IdleConnTimeout       time.Duration // Idle connection timeout (default: 90 seconds)
	TLSHandshakeTimeout   time.Duration // TLS handshake timeout (default: 10 seconds)
	ExpectContinueTimeout time.Duration // Expect 100-continue timeout (default: 1 second)

	// TLS configuration
	TLSConfig *tls.Config

	// Authentication configuration
	EnableJWTAuth bool   // Enable automatic JWT authentication
	APIKeyHeader  string // Header name for API key (default: "X-API-Key")

	// Secure credential provider (replaces plain text APIKey)
	CredentialProvider CredentialProvider // Provider for secure credential access

	// AllowedHosts optionally restricts requests (including RawRequest and
	// resolved BaseURL+path lookups) to this set of hosts. This is opt-in
	// defense-in-depth against SSRF for services that pass user-influenced
	// URLs or paths through the client. Empty/nil (default) preserves the
	// existing unrestricted behavior.
	AllowedHosts []string

	// Retry configuration
	RetryConfig RetryConfig

	// Circuit breaker configuration
	CircuitBreakerConfig CircuitBreakerConfig

	// Logging and observability
	EnableRequestLogging bool // Enable request/response logging
	EnableMetrics        bool // Enable request metrics collection
	LogRequestBody       bool // Log request bodies (be careful with sensitive data)
	LogResponseBody      bool // Log response bodies (be careful with sensitive data)
	MaxLogBodySize       int  // Maximum body size to log (default: 1024 bytes)

	// User agent
	UserAgent string // User agent string (default: "Forge-HTTP-Client/1.0")
}

// RetryConfig contains retry policy configuration.
type RetryConfig struct {
	MaxRetries          int           // Maximum number of retries (default: 3)
	InitialInterval     time.Duration // Initial retry interval (default: 100ms)
	MaxInterval         time.Duration // Maximum retry interval (default: 5s)
	Multiplier          float64       // Backoff multiplier (default: 2.0)
	RandomizationFactor float64       // Randomization factor (default: 0.1)
}

// CircuitBreakerConfig contains circuit breaker configuration.
type CircuitBreakerConfig struct {
	Name        string                             // Circuit breaker name for metrics
	MaxRequests uint32                             // Max requests in half-open state (default: 3)
	Interval    time.Duration                      // Interval to clear failure counts (default: 60s)
	Timeout     time.Duration                      // Timeout in open state (default: 30s)
	ReadyToTrip func(counts gobreaker.Counts) bool // Custom trip function
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Timeout:               30 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		APIKeyHeader:          "X-API-Key",
		EnableRequestLogging:  true,
		EnableMetrics:         true,
		LogRequestBody:        false,
		LogResponseBody:       false,
		MaxLogBodySize:        1024,
		UserAgent:             "Forge-HTTP-Client/1.0",
		RetryConfig: RetryConfig{
			MaxRetries:          3,
			InitialInterval:     100 * time.Millisecond,
			MaxInterval:         5 * time.Second,
			Multiplier:          2.0,
			RandomizationFactor: 0.1,
		},
		CircuitBreakerConfig: CircuitBreakerConfig{
			MaxRequests: 3,
			Interval:    60 * time.Second,
			Timeout:     30 * time.Second,
		},
	}
}

// Validate validates the HTTP client configuration.
func (c *Config) Validate() error {
	if c.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive, got %v", c.Timeout)
	}

	// Validate transport settings
	if c.MaxIdleConns <= 0 {
		return fmt.Errorf("max_idle_conns must be positive, got %d", c.MaxIdleConns)
	}
	if c.MaxIdleConnsPerHost <= 0 {
		return fmt.Errorf("max_idle_conns_per_host must be positive, got %d", c.MaxIdleConnsPerHost)
	}
	if c.MaxIdleConnsPerHost > c.MaxIdleConns {
		return fmt.Errorf("max_idle_conns_per_host (%d) cannot exceed max_idle_conns (%d)", c.MaxIdleConnsPerHost, c.MaxIdleConns)
	}

	// Validate timeouts
	if c.IdleConnTimeout <= 0 {
		return fmt.Errorf("idle_conn_timeout must be positive, got %v", c.IdleConnTimeout)
	}
	if c.TLSHandshakeTimeout <= 0 {
		return fmt.Errorf("tls_handshake_timeout must be positive, got %v", c.TLSHandshakeTimeout)
	}

	// Validate base URL if provided
	if c.BaseURL != "" {
		if _, err := url.Parse(c.BaseURL); err != nil {
			return fmt.Errorf("invalid base_url format: %w", err)
		}
	}

	// Validate retry configuration
	if c.RetryConfig.MaxRetries < 0 {
		return fmt.Errorf("max_retries must be non-negative, got %d", c.RetryConfig.MaxRetries)
	}
	if c.RetryConfig.InitialInterval <= 0 {
		return fmt.Errorf("initial_interval must be positive, got %v", c.RetryConfig.InitialInterval)
	}
	if c.RetryConfig.MaxInterval <= 0 {
		return fmt.Errorf("max_interval must be positive, got %v", c.RetryConfig.MaxInterval)
	}
	if c.RetryConfig.Multiplier <= 1.0 {
		return fmt.Errorf("multiplier must be greater than 1.0, got %f", c.RetryConfig.Multiplier)
	}

	// Validate circuit breaker configuration
	if c.CircuitBreakerConfig.MaxRequests == 0 {
		return stderrors.New("circuit breaker max_requests must be positive")
	}
	if c.CircuitBreakerConfig.Interval <= 0 {
		return fmt.Errorf("circuit breaker interval must be positive, got %v", c.CircuitBreakerConfig.Interval)
	}
	if c.CircuitBreakerConfig.Timeout <= 0 {
		return fmt.Errorf("circuit breaker timeout must be positive, got %v", c.CircuitBreakerConfig.Timeout)
	}

	// Validate logging configuration
	if c.MaxLogBodySize < 0 {
		return fmt.Errorf("max_log_body_size must be non-negative, got %d", c.MaxLogBodySize)
	}

	return nil
}

// Bundle provides HTTP client integration for Forge applications.
type Bundle struct {
	config         Config
	client         *Client
	circuitBreaker *gobreaker.CircuitBreaker
	logger         zerolog.Logger
}

// NewBundle creates a new HTTP client bundle.
func NewBundle(config Config) *Bundle {
	return &Bundle{
		config: config,
	}
}

// Name returns the bundle name.
func (b *Bundle) Name() string {
	return "http-client"
}

// Initialize sets up the HTTP client with all configured features.
func (b *Bundle) Initialize(app *framework.App) error {
	if err := b.config.Validate(); err != nil {
		return errors.ErrInvalidConfiguration.WithMessage("HTTP client configuration validation failed").WithCause(err)
	}

	if app != nil {
		b.logger = app.Logger().WithService("http-client", "httpclient")
	} else {
		b.logger = zerolog.Nop()
	}

	// Create HTTP transport with secure defaults
	transport := &http.Transport{
		MaxIdleConns:          b.config.MaxIdleConns,
		MaxIdleConnsPerHost:   b.config.MaxIdleConnsPerHost,
		IdleConnTimeout:       b.config.IdleConnTimeout,
		TLSHandshakeTimeout:   b.config.TLSHandshakeTimeout,
		ExpectContinueTimeout: b.config.ExpectContinueTimeout,
		DisableKeepAlives:     false,
		DisableCompression:    false,
		ResponseHeaderTimeout: b.config.Timeout / 2, // Prevent slow header attacks
	}

	// Configure secure TLS settings
	tlsConfig := b.config.TLSConfig
	if tlsConfig == nil {
		tlsConfig = &tls.Config{}
	}

	// Enforce secure TLS configuration
	tlsConfig.MinVersion = tls.VersionTLS12 // Require TLS 1.2 minimum
	tlsConfig.InsecureSkipVerify = false    // Never skip certificate verification

	// Set secure cipher suites
	tlsConfig.CipherSuites = []uint16{
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	}

	transport.TLSClientConfig = tlsConfig

	// Create circuit breaker
	cbSettings := gobreaker.Settings{
		Name:        b.config.CircuitBreakerConfig.Name,
		MaxRequests: b.config.CircuitBreakerConfig.MaxRequests,
		Interval:    b.config.CircuitBreakerConfig.Interval,
		Timeout:     b.config.CircuitBreakerConfig.Timeout,
	}

	// Use custom ReadyToTrip function if provided, otherwise use default
	if b.config.CircuitBreakerConfig.ReadyToTrip != nil {
		cbSettings.ReadyToTrip = b.config.CircuitBreakerConfig.ReadyToTrip
	} else {
		// Default: trip after 5 consecutive failures
		cbSettings.ReadyToTrip = func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		}
	}

	b.circuitBreaker = gobreaker.NewCircuitBreaker(cbSettings)

	// Create HTTP client
	httpClient := &http.Client{
		Transport:     transport,
		Timeout:       b.config.Timeout,
		CheckRedirect: redirectHeaderStripper(b.config.APIKeyHeader, b.config.AllowedHosts),
	}

	// Create enhanced client wrapper
	b.client = &Client{
		httpClient:     httpClient,
		config:         b.config,
		circuitBreaker: b.circuitBreaker,
		backoffFactory: b.createBackoffStrategy,
		logger:         b.logger,
	}

	return nil
}

// Client returns the enhanced HTTP client.
func (b *Bundle) Client() *Client {
	return b.client
}

// Stop implements the Bundle interface for graceful shutdown.
// Closes idle HTTP client connections respecting the context deadline.
func (b *Bundle) Stop(ctx context.Context) error {
	if b.client != nil && b.client.httpClient != nil && b.client.httpClient.Transport != nil {
		if transport, ok := b.client.httpClient.Transport.(*http.Transport); ok {
			// Close idle connections (this is a non-blocking operation)
			transport.CloseIdleConnections()
		}
	}
	return nil
}

// Close is deprecated. Use Stop() instead for proper lifecycle integration.
// Maintained for backward compatibility.
func (b *Bundle) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return b.Stop(ctx)
}

// maxRedirects matches the net/http default redirect limit.
const maxRedirects = 10

// redirectHeaderStripper returns an http.Client CheckRedirect function that
// removes credential headers whenever a redirect crosses to a different host,
// and, when allowedHosts is non-empty, rejects redirects to a host outside
// that allowlist. Go's default redirect handling already strips
// "Authorization" on cross-host redirects but forwards custom headers like
// the configured API-key header unchanged, allowing a malicious or
// compromised upstream to harvest credentials via a 302 to an
// attacker-controlled host. Same-host redirects are unaffected and continue
// to carry auth headers as before. The allowlist check closes a related gap:
// checkHostAllowed only validates the initial request URL, so without this,
// a redirect could still be followed to a disallowed host (with credentials
// stripped) even when AllowedHosts is configured.
func redirectHeaderStripper(apiKeyHeader string, allowedHosts []string) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("stopped after %d redirects", maxRedirects)
		}

		if !hostAllowed(req.URL.Hostname(), allowedHosts) {
			return fmt.Errorf("redirect to %w: %s", ErrHostNotAllowed, req.URL.Hostname())
		}

		prev := via[len(via)-1]
		if prev.URL.Host != req.URL.Host {
			req.Header.Del("Authorization")
			if apiKeyHeader != "" {
				req.Header.Del(apiKeyHeader)
			}
		}

		return nil
	}
}

// createBackoffStrategy creates the configured backoff strategy.
func (b *Bundle) createBackoffStrategy() backoff.BackOff {
	exponentialBackoff := backoff.NewExponentialBackOff()
	exponentialBackoff.InitialInterval = b.config.RetryConfig.InitialInterval
	exponentialBackoff.MaxInterval = b.config.RetryConfig.MaxInterval
	exponentialBackoff.Multiplier = b.config.RetryConfig.Multiplier
	exponentialBackoff.RandomizationFactor = b.config.RetryConfig.RandomizationFactor
	exponentialBackoff.MaxElapsedTime = 0 // No maximum elapsed time, use MaxRetries instead

	return backoff.WithMaxRetries(exponentialBackoff, uint64(b.config.RetryConfig.MaxRetries))
}

// Client provides enhanced HTTP client functionality with retries, circuit breaker, and observability.
type Client struct {
	httpClient     *http.Client
	config         Config
	circuitBreaker *gobreaker.CircuitBreaker
	backoffFactory func() backoff.BackOff
	logger         zerolog.Logger
}

// HTTPError represents an HTTP error response.
type HTTPError struct {
	StatusCode int    `json:"status_code"`
	Status     string `json:"status"`
	Body       string `json:"body,omitempty"`
	URL        string `json:"url"`
	Method     string `json:"method"`
}

// Error implements the error interface.
func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d %s: %s %s", e.StatusCode, e.Status, e.Method, e.URL)
}

// IsRetryableError checks if an HTTP error should be retried.
func (e *HTTPError) IsRetryableError() bool {
	// Retry on server errors and specific client errors that are transient
	switch e.StatusCode {
	case 408, // Request Timeout
		429, // Too Many Requests
		502, // Bad Gateway
		503, // Service Unavailable
		504, // Gateway Timeout
		520, // Unknown Error (Cloudflare)
		521, // Web Server Is Down (Cloudflare)
		522, // Connection Timed Out (Cloudflare)
		523, // Origin Is Unreachable (Cloudflare)
		524: // A Timeout Occurred (Cloudflare)
		return true
	}

	// Retry on all 5xx server errors except 501 (Not Implemented) and 505 (HTTP Version Not Supported)
	if e.StatusCode >= 500 && e.StatusCode != 501 && e.StatusCode != 505 {
		return true
	}

	return false
}

// Common errors
var (
	ErrCircuitBreakerOpen = stderrors.New("circuit breaker is open")
	ErrMaxRetriesExceeded = stderrors.New("maximum retries exceeded")
	ErrHostNotAllowed     = stderrors.New("host not allowed")
)

// Get performs a GET request and unmarshals the response into dest.
func (c *Client) Get(ctx context.Context, path string, dest interface{}) error {
	return c.request(ctx, http.MethodGet, path, nil, dest)
}

// Post performs a POST request with the given body and unmarshals the response into dest.
func (c *Client) Post(ctx context.Context, path string, body interface{}, dest interface{}) error {
	return c.request(ctx, http.MethodPost, path, body, dest)
}

// Put performs a PUT request with the given body and unmarshals the response into dest.
func (c *Client) Put(ctx context.Context, path string, body interface{}, dest interface{}) error {
	return c.request(ctx, http.MethodPut, path, body, dest)
}

// Delete performs a DELETE request.
func (c *Client) Delete(ctx context.Context, path string) error {
	return c.request(ctx, http.MethodDelete, path, nil, nil)
}

// request performs an HTTP request with retries, circuit breaker, and observability.
func (c *Client) request(ctx context.Context, method, path string, body interface{}, dest interface{}) error {
	// Build full URL
	fullURL, err := c.buildURL(path)
	if err != nil {
		return fmt.Errorf("failed to build URL: %w", err)
	}

	if err := c.checkHostAllowed(fullURL); err != nil {
		return err
	}

	// Execute request with circuit breaker protection
	_, err = c.circuitBreaker.Execute(func() (interface{}, error) {
		return nil, c.executeWithRetry(ctx, method, fullURL, body, dest)
	})

	if err != nil {
		// Check if circuit breaker is open
		if stderrors.Is(err, gobreaker.ErrOpenState) {
			return ErrCircuitBreakerOpen
		}
		return err
	}

	return nil
}

// executeWithRetry executes an HTTP request with retry logic.
func (c *Client) executeWithRetry(ctx context.Context, method, url string, body interface{}, dest interface{}) error {
	operation := func() error {
		return c.executeRequest(ctx, method, url, body, dest)
	}

	// Create a fresh backoff instance per request to avoid shared mutable state
	b := backoff.WithContext(c.backoffFactory(), ctx)

	// Retry with backoff
	lastErr := backoff.Retry(operation, b)
	if lastErr != nil {
		// Distinguish context cancellation/deadline from genuine retry exhaustion
		// so callers can detect cancellation instead of misreading it as exhaustion.
		if stderrors.Is(lastErr, context.Canceled) || stderrors.Is(lastErr, context.DeadlineExceeded) {
			return fmt.Errorf("request context ended: %w", lastErr)
		}
		return fmt.Errorf("%w: %w", ErrMaxRetriesExceeded, lastErr)
	}

	return nil
}

// executeRequest executes a single HTTP request with logging and metrics.
func (c *Client) executeRequest(ctx context.Context, method, url string, body interface{}, dest interface{}) error {
	start := time.Now()

	// Prepare request body
	var bodyReader io.Reader
	var contentType string
	var requestBody []byte

	if body != nil {
		var err error
		requestBody, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(requestBody)
		contentType = "application/json"
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("User-Agent", c.config.UserAgent)

	// Add authentication headers
	if err := c.addAuthHeaders(ctx, req); err != nil {
		return fmt.Errorf("failed to add authentication headers: %w", err)
	}

	// Log request if enabled
	if c.config.EnableRequestLogging {
		c.logRequest(req, requestBody)
	}

	// Execute request
	resp, err := c.httpClient.Do(req)
	duration := time.Since(start)

	if err != nil {
		// Log error
		if c.config.EnableRequestLogging {
			c.logRequestError(method, url, duration, err)
		}

		// Check if error is retryable
		var netErr net.Error
		if stderrors.As(err, &netErr) && netErr.Timeout() {
			return err // Retryable timeout error
		}
		return backoff.Permanent(err) // Non-retryable error
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Log response if enabled
	if c.config.EnableRequestLogging {
		c.logResponse(resp, respBody, duration)
	}

	// Check for HTTP errors
	if resp.StatusCode >= 400 {
		httpErr := &HTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       string(respBody),
			URL:        url,
			Method:     method,
		}

		// Return permanent error for client errors (don't retry)
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 408 && resp.StatusCode != 429 {
			return backoff.Permanent(httpErr)
		}

		return httpErr // Retryable server error
	}

	// Unmarshal response if destination provided
	if dest != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, dest); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return nil
}

// logRequest logs HTTP request details with security filtering.
func (c *Client) logRequest(req *http.Request, body []byte) {
	// Enforce maximum log body size to prevent memory exhaustion
	maxSize := c.config.MaxLogBodySize
	if maxSize <= 0 || maxSize > 4096 { // Hard limit: 4KB max for security
		maxSize = 1024
	}

	logBody := ""
	if c.config.LogRequestBody && len(body) > 0 {
		// Limit body size to prevent memory exhaustion
		if len(body) > maxSize {
			logBody = fmt.Sprintf("[TRUNCATED %d bytes] %s...", len(body), string(body[:maxSize]))
		} else {
			logBody = string(body)
		}
	}

	// Filter sensitive headers from logging
	safeURL := c.sanitizeURLForLogging(req.URL.String())

	c.logger.Debug().
		Str("method", req.Method).
		Str("url", safeURL).
		Str("body", logBody).
		Msg("HTTP Request")
}

// logResponse logs HTTP response details with security filtering.
func (c *Client) logResponse(resp *http.Response, body []byte, duration time.Duration) {
	// Enforce maximum log body size to prevent memory exhaustion
	maxSize := c.config.MaxLogBodySize
	if maxSize <= 0 || maxSize > 4096 { // Hard limit: 4KB max for security
		maxSize = 1024
	}

	logBody := ""
	if c.config.LogResponseBody && len(body) > 0 {
		// Limit body size to prevent memory exhaustion
		if len(body) > maxSize {
			logBody = fmt.Sprintf("[TRUNCATED %d bytes] %s...", len(body), string(body[:maxSize]))
		} else {
			logBody = string(body)
		}
	}

	c.logger.Debug().
		Int("status_code", resp.StatusCode).
		Str("status", resp.Status).
		Dur("duration", duration).
		Str("body", logBody).
		Msg("HTTP Response")
}

// sanitizeURLForLogging removes sensitive query parameters from URLs for logging.
func (c *Client) sanitizeURLForLogging(rawURL string) string {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "[invalid-url]"
	}

	// Remove sensitive query parameters
	query := parsedURL.Query()
	sensitiveParams := []string{"token", "key", "secret", "password", "auth", "api_key"}

	for _, param := range sensitiveParams {
		if query.Has(param) {
			query.Set(param, "[REDACTED]")
		}
	}

	parsedURL.RawQuery = query.Encode()
	return parsedURL.String()
}

// logRequestError logs HTTP request errors.
func (c *Client) logRequestError(method, url string, duration time.Duration, err error) {
	c.logger.Error().
		Str("method", method).
		Str("url", c.sanitizeURLForLogging(url)).
		Dur("duration", duration).
		Err(err).
		Msg("HTTP Request Error")
}

// buildURL constructs the full URL from base URL and path.
func (c *Client) buildURL(path string) (string, error) {
	if c.config.BaseURL == "" {
		return path, nil
	}

	baseURL, err := url.Parse(c.config.BaseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}

	pathURL, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	return baseURL.ResolveReference(pathURL).String(), nil
}

// checkHostAllowed validates rawURL's host against config.AllowedHosts. It is
// an opt-in SSRF safeguard: when AllowedHosts is empty (the default), every
// host is permitted, preserving existing behavior. When non-empty, requests
// to any other host are rejected with ErrHostNotAllowed. This covers both the
// shared request path (buildURL results, which may be overridden by an
// absolute path) and RawRequest, where the caller supplies the URL directly.
// Redirect targets are separately enforced by redirectHeaderStripper via the
// same hostAllowed helper, since this check only sees the initial URL.
func (c *Client) checkHostAllowed(rawURL string) error {
	if len(c.config.AllowedHosts) == 0 {
		return nil
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	host := parsedURL.Hostname()
	if !hostAllowed(host, c.config.AllowedHosts) {
		return fmt.Errorf("%w: %s", ErrHostNotAllowed, host)
	}

	return nil
}

// hostAllowed reports whether host is permitted under allowedHosts. An empty
// allowedHosts means unrestricted (everything is allowed), matching the
// opt-in nature of the AllowedHosts config field.
func hostAllowed(host string, allowedHosts []string) bool {
	if len(allowedHosts) == 0 {
		return true
	}
	for _, allowed := range allowedHosts {
		if host == allowed {
			return true
		}
	}
	return false
}

// addAuthHeaders adds authentication headers to the request.
func (c *Client) addAuthHeaders(ctx context.Context, req *http.Request) error {
	// Add API key if credential provider is configured
	if c.config.CredentialProvider != nil {
		apiKey, err := c.config.CredentialProvider.GetAPIKey(ctx)
		if err == nil && apiKey != "" {
			req.Header.Set(c.config.APIKeyHeader, apiKey)
		}
		// Don't fail the request if API key is not available - might be using JWT auth
	}

	// Add JWT token if JWT authentication is enabled
	if c.config.EnableJWTAuth {
		// First try to get JWT token from context (manual injection)
		if token := getJWTTokenFromContext(ctx); token != "" {
			if err := c.validateJWTToken(token); err != nil {
				return fmt.Errorf("invalid JWT token from context: %w", err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
		} else if c.config.CredentialProvider != nil {
			// Fallback to credential provider
			token, err := c.config.CredentialProvider.GetJWTToken(ctx)
			if err == nil && token != "" {
				if err := c.validateJWTToken(token); err != nil {
					return fmt.Errorf("invalid JWT token from provider: %w", err)
				}
				req.Header.Set("Authorization", "Bearer "+token)
			}
		}
	}

	return nil
}

// validateJWTToken performs basic JWT token validation.
func (c *Client) validateJWTToken(token string) error {
	if token == "" {
		return stderrors.New("token cannot be empty")
	}

	// Basic JWT format validation (header.payload.signature)
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return stderrors.New("invalid JWT token format")
	}

	// Additional validation would be performed by JWT bundle
	return nil
}

// jwtTokenContextKey is used to store JWT tokens in context for HTTP client use.
type jwtTokenContextKey struct{}

// WithJWTToken adds a JWT token to the context for HTTP client authentication.
func WithJWTToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, jwtTokenContextKey{}, token)
}

// getJWTTokenFromContext extracts JWT token from context.
func getJWTTokenFromContext(ctx context.Context) string {
	if token, ok := ctx.Value(jwtTokenContextKey{}).(string); ok {
		return token
	}
	return ""
}

// EnableJWTIntegration creates a client option that automatically injects JWT tokens.
func (b *Bundle) EnableJWTIntegration(jwtBundle interface{}) error {
	// This would be called during bundle initialization to integrate with JWT bundle
	// For now, JWT tokens can be manually added to context using WithJWTToken
	b.config.EnableJWTAuth = true
	return nil
}

// RawRequest performs a raw HTTP request with full control over request/response.
func (c *Client) RawRequest(ctx context.Context, method, url string, headers map[string]string, body io.Reader) (*http.Response, error) {
	if err := c.checkHostAllowed(url); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set custom headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Set default user agent if not provided
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", c.config.UserAgent)
	}

	// Add authentication headers
	if err := c.addAuthHeaders(ctx, req); err != nil {
		return nil, fmt.Errorf("failed to add authentication headers: %w", err)
	}

	// Execute with circuit breaker but without automatic retries
	result, err := c.circuitBreaker.Execute(func() (interface{}, error) {
		return c.httpClient.Do(req)
	})

	if err != nil {
		if stderrors.Is(err, gobreaker.ErrOpenState) {
			return nil, ErrCircuitBreakerOpen
		}
		return nil, err
	}

	return result.(*http.Response), nil
}

// HealthCheck performs health checks for the HTTP client.
func (c *Client) HealthCheck(ctx context.Context, healthURL string) error {
	if healthURL == "" {
		return stderrors.New("health check URL not configured")
	}

	// Perform a simple GET request to health endpoint
	resp, err := c.RawRequest(ctx, http.MethodGet, healthURL, map[string]string{
		"Accept": "application/json",
	}, nil)

	if err != nil {
		return fmt.Errorf("health check request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: HTTP %d %s", resp.StatusCode, resp.Status)
	}

	return nil
}

// GetCircuitBreakerState returns the current circuit breaker state.
func (c *Client) GetCircuitBreakerState() gobreaker.State {
	return c.circuitBreaker.State()
}

// GetCircuitBreakerCounts returns the current circuit breaker counts.
func (c *Client) GetCircuitBreakerCounts() gobreaker.Counts {
	return c.circuitBreaker.Counts()
}
