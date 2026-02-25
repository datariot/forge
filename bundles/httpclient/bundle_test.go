package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestDefaultConfig tests default HTTP client configuration
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Timeout != 30*time.Second {
		t.Errorf("Expected Timeout 30s, got %v", cfg.Timeout)
	}

	if cfg.MaxIdleConns != 100 {
		t.Errorf("Expected MaxIdleConns 100, got %d", cfg.MaxIdleConns)
	}

	if cfg.MaxIdleConnsPerHost != 10 {
		t.Errorf("Expected MaxIdleConnsPerHost 10, got %d", cfg.MaxIdleConnsPerHost)
	}

	if cfg.IdleConnTimeout != 90*time.Second {
		t.Errorf("Expected IdleConnTimeout 90s, got %v", cfg.IdleConnTimeout)
	}

	// Check retry config defaults
	if cfg.RetryConfig.MaxRetries != 3 {
		t.Errorf("Expected default MaxRetries 3, got %d", cfg.RetryConfig.MaxRetries)
	}

	if cfg.RetryConfig.InitialInterval != 100*time.Millisecond {
		t.Errorf("Expected default InitialInterval 100ms, got %v", cfg.RetryConfig.InitialInterval)
	}

	// Check circuit breaker defaults
	if cfg.CircuitBreakerConfig.MaxRequests != 3 {
		t.Errorf("Expected default CB MaxRequests 3, got %d", cfg.CircuitBreakerConfig.MaxRequests)
	}

	if cfg.CircuitBreakerConfig.Timeout != 30*time.Second {
		t.Errorf("Expected default CB Timeout 30s, got %v", cfg.CircuitBreakerConfig.Timeout)
	}
}

// TestNewBundle tests bundle creation
func TestNewBundle(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseURL = "http://api.example.com"

	bundle := NewBundle(cfg)

	if bundle == nil {
		t.Fatal("Expected bundle to be created")
	}

	if bundle.config.BaseURL != cfg.BaseURL {
		t.Error("Expected config to be set")
	}
}

// TestBundle_Name tests bundle name
func TestBundle_Name(t *testing.T) {
	bundle := NewBundle(DefaultConfig())

	if bundle.Name() != "http-client" {
		t.Errorf("Expected bundle name 'http-client', got %s", bundle.Name())
	}
}

// TestBundle_Initialize tests bundle initialization
func TestBundle_Initialize(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseURL = "http://api.example.com"

	bundle := NewBundle(cfg)

	// Initialize should succeed even without base URL (client can work without it)
	if err := bundle.Initialize(nil); err != nil {
		t.Errorf("Expected initialization to succeed, got: %v", err)
	}

	// Verify client was created
	if bundle.Client() == nil {
		t.Error("Expected client to be created after initialization")
	}
}

// TestBundle_Client_BeforeInitialize tests Client getter before initialization
func TestBundle_Client_BeforeInitialize(t *testing.T) {
	bundle := NewBundle(DefaultConfig())

	// Before initialization
	if bundle.Client() != nil {
		t.Error("Expected Client to be nil before initialization")
	}
}

// TestBundle_Stop tests Stop method
func TestBundle_Stop(t *testing.T) {
	cfg := DefaultConfig()
	bundle := NewBundle(cfg)

	// Initialize first
	bundle.Initialize(nil)

	ctx := context.Background()
	if err := bundle.Stop(ctx); err != nil {
		t.Errorf("Expected Stop to succeed, got: %v", err)
	}
}

// TestBundle_Stop_BeforeInitialize tests Stop before initialization
func TestBundle_Stop_BeforeInitialize(t *testing.T) {
	bundle := NewBundle(DefaultConfig())

	ctx := context.Background()
	if err := bundle.Stop(ctx); err != nil {
		t.Errorf("Expected Stop to succeed even before initialization, got: %v", err)
	}
}

// TestConfig_RetrySettings tests retry configuration embedded in Config
func TestConfig_RetrySettings(t *testing.T) {
	cfg := DefaultConfig()

	// Verify retry settings are initialized
	if cfg.RetryConfig.MaxRetries <= 0 {
		t.Error("Expected positive MaxRetries")
	}

	if cfg.RetryConfig.InitialInterval <= 0 {
		t.Error("Expected positive InitialInterval")
	}

	if cfg.RetryConfig.MaxInterval <= 0 {
		t.Error("Expected positive MaxInterval")
	}

	if cfg.RetryConfig.Multiplier <= 1.0 {
		t.Error("Expected Multiplier > 1.0 for exponential backoff")
	}
}

// TestConfig_CircuitBreakerSettings tests circuit breaker configuration
func TestConfig_CircuitBreakerSettings(t *testing.T) {
	cfg := DefaultConfig()

	// Verify circuit breaker settings are initialized
	if cfg.CircuitBreakerConfig.MaxRequests < 1 {
		t.Error("MaxRequests should be at least 1")
	}

	if cfg.CircuitBreakerConfig.Interval < 1*time.Second {
		t.Error("Interval should be at least 1 second")
	}

	if cfg.CircuitBreakerConfig.Timeout < 1*time.Second {
		t.Error("Timeout should be at least 1 second")
	}
}

// TestConfig_HTTPSecurity tests HTTPS-related configuration
func TestConfig_HTTPSecurity(t *testing.T) {
	cfg := DefaultConfig()

	// Verify secure defaults
	if cfg.MaxIdleConns <= 0 {
		t.Error("MaxIdleConns should be positive")
	}

	if cfg.MaxIdleConnsPerHost <= 0 {
		t.Error("MaxIdleConnsPerHost should be positive")
	}

	if cfg.IdleConnTimeout <= 0 {
		t.Error("IdleConnTimeout should be positive")
	}

	if cfg.Timeout <= 0 {
		t.Error("Timeout should be positive")
	}
}

// --- Validate error paths ---

func TestValidate_NonPositiveTimeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Timeout = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for zero timeout")
	}
}

func TestValidate_NonPositiveMaxIdleConns(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxIdleConns = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for zero MaxIdleConns")
	}
}

func TestValidate_MaxIdleConnsPerHostExceedsMax(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxIdleConnsPerHost = cfg.MaxIdleConns + 1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error when MaxIdleConnsPerHost > MaxIdleConns")
	}
}

func TestValidate_NonPositiveIdleConnTimeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.IdleConnTimeout = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for zero IdleConnTimeout")
	}
}

func TestValidate_NonPositiveTLSHandshakeTimeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TLSHandshakeTimeout = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for zero TLSHandshakeTimeout")
	}
}

func TestValidate_NegativeRetryMaxRetries(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RetryConfig.MaxRetries = -1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative MaxRetries")
	}
}

func TestValidate_ZeroRetryInitialInterval(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RetryConfig.InitialInterval = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for zero InitialInterval")
	}
}

func TestValidate_ZeroRetryMaxInterval(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RetryConfig.MaxInterval = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for zero MaxInterval")
	}
}

func TestValidate_MultiplierNotGreaterThanOne(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RetryConfig.Multiplier = 1.0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for multiplier <= 1.0")
	}
}

func TestValidate_ZeroCircuitBreakerMaxRequests(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CircuitBreakerConfig.MaxRequests = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for zero circuit breaker MaxRequests")
	}
}

func TestValidate_ZeroCircuitBreakerInterval(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CircuitBreakerConfig.Interval = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for zero circuit breaker Interval")
	}
}

func TestValidate_ZeroCircuitBreakerTimeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CircuitBreakerConfig.Timeout = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for zero circuit breaker Timeout")
	}
}

func TestValidate_NegativeMaxLogBodySize(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxLogBodySize = -1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative MaxLogBodySize")
	}
}

// --- StaticCredentialProvider tests ---

func TestStaticCredentialProvider_GetAPIKey(t *testing.T) {
	provider := NewStaticCredentialProvider("my-api-key", "")
	key, err := provider.GetAPIKey(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "my-api-key" {
		t.Errorf("expected 'my-api-key', got %q", key)
	}
}

func TestStaticCredentialProvider_GetAPIKey_Empty(t *testing.T) {
	provider := NewStaticCredentialProvider("", "")
	_, err := provider.GetAPIKey(context.Background())
	if err == nil {
		t.Error("expected error for empty API key")
	}
}

func TestStaticCredentialProvider_GetJWTToken(t *testing.T) {
	provider := NewStaticCredentialProvider("", "my.jwt.token")
	token, err := provider.GetJWTToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "my.jwt.token" {
		t.Errorf("expected 'my.jwt.token', got %q", token)
	}
}

func TestStaticCredentialProvider_GetJWTToken_Empty(t *testing.T) {
	provider := NewStaticCredentialProvider("", "")
	_, err := provider.GetJWTToken(context.Background())
	if err == nil {
		t.Error("expected error for empty JWT token")
	}
}

// --- Bundle extras ---

func TestBundle_EnableJWTIntegration(t *testing.T) {
	b := NewBundle(DefaultConfig())
	if err := b.EnableJWTIntegration(nil); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !b.config.EnableJWTAuth {
		t.Error("expected EnableJWTAuth=true after EnableJWTIntegration")
	}
}

func TestBundle_Close(t *testing.T) {
	b := NewBundle(DefaultConfig())
	if err := b.Close(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- HTTPError tests ---

func TestHTTPError_Error(t *testing.T) {
	err := &HTTPError{
		StatusCode: 404,
		Status:     "404 Not Found",
		Method:     "GET",
		URL:        "https://example.com/api",
	}
	msg := err.Error()
	if msg == "" {
		t.Error("expected non-empty error message")
	}
}

func TestHTTPError_IsRetryableError(t *testing.T) {
	tests := []struct {
		statusCode int
		retryable  bool
	}{
		{200, false},
		{400, false},
		{404, false},
		{408, true},
		{429, true},
		{500, true},
		{502, true},
		{503, true},
		{504, true},
		{501, false},
		{505, false},
	}
	for _, tt := range tests {
		err := &HTTPError{StatusCode: tt.statusCode}
		if got := err.IsRetryableError(); got != tt.retryable {
			t.Errorf("StatusCode %d: expected retryable=%v, got %v", tt.statusCode, tt.retryable, got)
		}
	}
}

// --- WithJWTToken / getJWTTokenFromContext tests ---

func TestWithJWTToken_RoundTrip(t *testing.T) {
	ctx := context.Background()
	ctx = WithJWTToken(ctx, "header.payload.signature")

	token := getJWTTokenFromContext(ctx)
	if token != "header.payload.signature" {
		t.Errorf("expected token 'header.payload.signature', got %q", token)
	}
}

func TestGetJWTTokenFromContext_Missing(t *testing.T) {
	token := getJWTTokenFromContext(context.Background())
	if token != "" {
		t.Errorf("expected empty token for context without JWT, got %q", token)
	}
}

// --- Client integration tests using test HTTP server ---

func newTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	cfg := DefaultConfig()
	cfg.BaseURL = baseURL
	cfg.RetryConfig.MaxRetries = 0
	cfg.RetryConfig.InitialInterval = 10 * time.Millisecond
	cfg.RetryConfig.MaxInterval = 100 * time.Millisecond
	cfg.CircuitBreakerConfig.Name = "test"

	b := NewBundle(cfg)
	// Use Initialize(nil) to build the client (app=nil uses zerolog.Nop)
	if err := b.Initialize(nil); err != nil {
		t.Fatalf("failed to initialize bundle: %v", err)
	}
	// Override base URL after init since Initialize copies config into client
	b.client.config.BaseURL = baseURL
	return b.client
}

func TestClient_Get_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"name":"test"}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	var result map[string]string
	if err := client.Get(context.Background(), "/api/test", &result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["name"] != "test" {
		t.Errorf("expected name='test', got %q", result["name"])
	}
}

func TestClient_Get_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	var result map[string]string
	err := client.Get(context.Background(), "/api/missing", &result)
	if err == nil {
		t.Error("expected error for 404 response")
	}
}

func TestClient_Post_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"123"}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	body := map[string]string{"name": "test"}
	var result map[string]string
	if err := client.Post(context.Background(), "/api/items", body, &result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["id"] != "123" {
		t.Errorf("expected id='123', got %q", result["id"])
	}
}

func TestClient_Delete_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	if err := client.Delete(context.Background(), "/api/items/123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_RawRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom-Header") != "value" {
			t.Error("expected custom header to be set")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(t, "")

	resp, err := client.RawRequest(context.Background(), http.MethodGet, server.URL, map[string]string{
		"X-Custom-Header": "value",
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestClient_HealthCheck_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(t, "")
	if err := client.HealthCheck(context.Background(), server.URL+"/health"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_HealthCheck_EmptyURL(t *testing.T) {
	client := newTestClient(t, "")
	if err := client.HealthCheck(context.Background(), ""); err == nil {
		t.Error("expected error for empty health URL")
	}
}

func TestClient_GetCircuitBreakerState(t *testing.T) {
	client := newTestClient(t, "")
	state := client.GetCircuitBreakerState()
	_ = state
}

func TestClient_GetCircuitBreakerCounts(t *testing.T) {
	client := newTestClient(t, "")
	counts := client.GetCircuitBreakerCounts()
	_ = counts
}

func TestClient_Put_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"updated":true}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	type Response struct {
		Updated bool `json:"updated"`
	}
	var resp Response
	err := client.Put(context.Background(), "/resource", map[string]string{"key": "value"}, &resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Updated {
		t.Error("expected Updated=true")
	}
}
