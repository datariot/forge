package framework

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- HTTPServerConfig.Validate tests ---

func TestHTTPServerConfig_Validate_Defaults(t *testing.T) {
	cfg := DefaultHTTPServerConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid default config, got: %v", err)
	}
}

func TestHTTPServerConfig_Validate_CORSNoOrigins(t *testing.T) {
	cfg := DefaultHTTPServerConfig()
	cfg.EnableCORS = true
	cfg.CORSOrigins = []string{}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for CORS enabled with no origins")
	}
}

func TestHTTPServerConfig_Validate_WildcardWithCredentials(t *testing.T) {
	cfg := DefaultHTTPServerConfig()
	cfg.EnableCORS = true
	cfg.CORSOrigins = []string{"*"}
	cfg.CORSCredentials = true
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for wildcard CORS with credentials")
	}
}

func TestHTTPServerConfig_Validate_CORSNoMethods(t *testing.T) {
	cfg := DefaultHTTPServerConfig()
	cfg.EnableCORS = true
	cfg.CORSOrigins = []string{"https://example.com"}
	cfg.CORSMethods = []string{}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for CORS with no methods")
	}
}

func TestHTTPServerConfig_Validate_EmptyMetricsPath(t *testing.T) {
	cfg := DefaultHTTPServerConfig()
	cfg.MetricsPath = ""
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for empty metrics path")
	}
}

func TestHTTPServerConfig_Validate_EmptyHealthPrefix(t *testing.T) {
	cfg := DefaultHTTPServerConfig()
	cfg.HealthPathPrefix = ""
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for empty health path prefix")
	}
}

func TestHTTPServerConfig_Validate_ConflictingPaths(t *testing.T) {
	cfg := DefaultHTTPServerConfig()
	cfg.MetricsPath = "/health"
	cfg.HealthPathPrefix = "/health"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for conflicting metrics and health paths")
	}
}

func TestHTTPServerConfig_Validate_ValidCORS(t *testing.T) {
	cfg := DefaultHTTPServerConfig()
	cfg.EnableCORS = true
	cfg.CORSOrigins = []string{"https://example.com", "https://app.example.com"}
	cfg.CORSCredentials = true
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid CORS config, got: %v", err)
	}
}

func TestHTTPServerConfig_Validate_WildcardWithoutCredentials(t *testing.T) {
	cfg := DefaultHTTPServerConfig()
	cfg.EnableCORS = true
	cfg.CORSOrigins = []string{"*"}
	cfg.CORSCredentials = false
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected wildcard CORS without credentials to be valid, got: %v", err)
	}
}

// --- responseWriter tests ---

func TestResponseWriter_DefaultStatusCode(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	if rw.statusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", rw.statusCode)
	}
}

func TestResponseWriter_WriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	rw.WriteHeader(http.StatusNotFound)
	if rw.statusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rw.statusCode)
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected recorder status 404, got %d", rec.Code)
	}
}

func TestResponseWriter_Write_SetsStatusOK(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: 0}

	n, err := rw.Write([]byte("hello"))
	if err != nil {
		t.Errorf("unexpected write error: %v", err)
	}
	if n != 5 {
		t.Errorf("expected 5 bytes written, got %d", n)
	}
	if rw.statusCode != http.StatusOK {
		t.Errorf("expected status 200 after Write(), got %d", rw.statusCode)
	}
}

func TestResponseWriter_Write_PreservesExistingStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusCreated}

	rw.Write([]byte("data"))
	if rw.statusCode != http.StatusCreated {
		t.Errorf("expected status 201 preserved, got %d", rw.statusCode)
	}
}

// --- isAllowedOrigin tests (via corsMiddleware) ---

func TestCORSMiddleware_AllowedOrigin(t *testing.T) {
	builder := &HTTPServerBuilder{
		config: HTTPServerConfig{
			EnableCORS:      true,
			CORSOrigins:     []string{"https://example.com"},
			CORSMethods:     []string{"GET", "POST"},
			CORSHeaders:     []string{"Content-Type"},
			CORSCredentials: false,
		},
	}

	// Wrap a simple OK handler with CORS middleware
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := builder.corsMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "https://example.com" {
		t.Errorf("expected origin header 'https://example.com', got %q", origin)
	}
}

func TestCORSMiddleware_WildcardOrigin(t *testing.T) {
	builder := &HTTPServerBuilder{
		config: HTTPServerConfig{
			EnableCORS:      true,
			CORSOrigins:     []string{"*"},
			CORSMethods:     []string{"GET"},
			CORSHeaders:     []string{"Content-Type"},
			CORSCredentials: false,
		},
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := builder.corsMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Origin", "https://any.example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
		t.Errorf("expected wildcard origin header, got %q", origin)
	}
}

func TestCORSMiddleware_UnknownOrigin(t *testing.T) {
	builder := &HTTPServerBuilder{
		config: HTTPServerConfig{
			EnableCORS:      true,
			CORSOrigins:     []string{"https://allowed.com"},
			CORSMethods:     []string{"GET"},
			CORSHeaders:     []string{"Content-Type"},
			CORSCredentials: false,
		},
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := builder.corsMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Origin", "https://evil.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Request still succeeds (CORS doesn't block at server, browser enforces)
	// but origin header is not set
	if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "" {
		t.Errorf("expected no origin header for disallowed origin, got %q", origin)
	}
}

func TestCORSMiddleware_PreflightRequest(t *testing.T) {
	builder := &HTTPServerBuilder{
		config: HTTPServerConfig{
			EnableCORS:      true,
			CORSOrigins:     []string{"https://example.com"},
			CORSMethods:     []string{"GET", "POST"},
			CORSHeaders:     []string{"Content-Type"},
			CORSCredentials: false,
		},
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := builder.corsMiddleware(inner)

	req := httptest.NewRequest(http.MethodOptions, "/api", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for preflight, got %d", w.Code)
	}
}

func TestCORSMiddleware_WithCredentials(t *testing.T) {
	builder := &HTTPServerBuilder{
		config: HTTPServerConfig{
			EnableCORS:      true,
			CORSOrigins:     []string{"https://example.com"},
			CORSMethods:     []string{"GET"},
			CORSHeaders:     []string{"Content-Type"},
			CORSCredentials: true,
		},
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := builder.corsMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if creds := w.Header().Get("Access-Control-Allow-Credentials"); creds != "true" {
		t.Errorf("expected credentials header 'true', got %q", creds)
	}
}

func TestCORSMiddleware_NoOriginHeader(t *testing.T) {
	builder := &HTTPServerBuilder{
		config: HTTPServerConfig{
			EnableCORS:      true,
			CORSOrigins:     []string{"https://example.com"},
			CORSMethods:     []string{"GET"},
			CORSHeaders:     []string{"Content-Type"},
			CORSCredentials: false,
		},
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := builder.corsMiddleware(inner)

	// Request without Origin header (same-origin or non-browser)
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for request without Origin, got %d", w.Code)
	}
}

// --- promLogAdapter tests ---

func TestPromLogAdapter_Println(t *testing.T) {
	// Just verify it doesn't panic
	adapter := &promLogAdapter{}
	adapter.Println("test", "error", "message")
}

// --- DefaultHTTPServerConfig tests ---

func TestDefaultHTTPServerConfig(t *testing.T) {
	cfg := DefaultHTTPServerConfig()

	if cfg.EnableCORS {
		t.Error("CORS should be disabled by default")
	}
	if !cfg.EnableMetrics {
		t.Error("metrics should be enabled by default")
	}
	if cfg.MetricsPath != "/metrics" {
		t.Errorf("expected /metrics, got %q", cfg.MetricsPath)
	}
	if cfg.HealthPathPrefix != "/health" {
		t.Errorf("expected /health, got %q", cfg.HealthPathPrefix)
	}
	if !cfg.EnableRequestLogging {
		t.Error("request logging should be enabled by default")
	}
}
