package framework

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/datariot/forge/config"
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

	_, _ = rw.Write([]byte("data"))
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

func TestCORSMiddleware_ReflectedOrigin_SetsVaryOrigin(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !containsValue(w.Header().Values("Vary"), "Origin") {
		t.Errorf("expected Vary header to include Origin, got %v", w.Header().Values("Vary"))
	}
}

func TestCORSMiddleware_ReflectedOrigin_AppendsToExistingVary(t *testing.T) {
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
		w.Header().Add("Vary", "Accept-Encoding")
		w.WriteHeader(http.StatusOK)
	})
	handler := builder.corsMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	vary := w.Header().Values("Vary")
	if !containsValue(vary, "Origin") || !containsValue(vary, "Accept-Encoding") {
		t.Errorf("expected Vary to contain both Origin and Accept-Encoding, got %v", vary)
	}
}

func TestCORSMiddleware_WildcardOrigin_NoVaryHeader(t *testing.T) {
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

	if vary := w.Header().Get("Vary"); vary != "" {
		t.Errorf("expected no Vary header for wildcard origin, got %q", vary)
	}
}

func containsValue(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

// --- Metrics basic auth tests ---

func newTestBuilder(t *testing.T, env string, httpCfg HTTPServerConfig) *HTTPServerBuilder {
	t.Helper()
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "http-test"
	cfg.AppEnv = env

	app, err := New(WithConfig(&cfg))
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	return NewHTTPServerBuilder(app, httpCfg)
}

func TestRegisterMetricsEndpoint_OpenWhenAuthUnset(t *testing.T) {
	builder := newTestBuilder(t, "development", DefaultHTTPServerConfig())
	builder.registerMetricsEndpoint()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	builder.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected metrics endpoint to be open when credentials are unset, got %d", w.Code)
	}
}

func TestRegisterMetricsEndpoint_RejectsWrongCredentials(t *testing.T) {
	httpCfg := DefaultHTTPServerConfig()
	httpCfg.MetricsBasicAuthUser = "prom"
	httpCfg.MetricsBasicAuthPass = "s3cret"

	builder := newTestBuilder(t, "development", httpCfg)
	builder.registerMetricsEndpoint()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.SetBasicAuth("prom", "wrong-password")
	w := httptest.NewRecorder()
	builder.mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong credentials, got %d", w.Code)
	}
}

func TestRegisterMetricsEndpoint_RejectsMissingCredentials(t *testing.T) {
	httpCfg := DefaultHTTPServerConfig()
	httpCfg.MetricsBasicAuthUser = "prom"
	httpCfg.MetricsBasicAuthPass = "s3cret"

	builder := newTestBuilder(t, "development", httpCfg)
	builder.registerMetricsEndpoint()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	builder.mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with no credentials, got %d", w.Code)
	}
}

func TestRegisterMetricsEndpoint_AcceptsCorrectCredentials(t *testing.T) {
	httpCfg := DefaultHTTPServerConfig()
	httpCfg.MetricsBasicAuthUser = "prom"
	httpCfg.MetricsBasicAuthPass = "s3cret"

	builder := newTestBuilder(t, "development", httpCfg)
	builder.registerMetricsEndpoint()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.SetBasicAuth("prom", "s3cret")
	w := httptest.NewRecorder()
	builder.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for correct credentials, got %d", w.Code)
	}
}

func TestConstantTimeEqual(t *testing.T) {
	if !constantTimeEqual("secret", "secret") {
		t.Error("expected equal strings to compare equal")
	}
	if constantTimeEqual("secret", "different") {
		t.Error("expected different strings to compare unequal")
	}
	if constantTimeEqual("secret", "secretlonger") {
		t.Error("expected different-length strings to compare unequal")
	}
}

// --- Pprof environment gating tests ---

func TestRegisterPprofEndpoints_BlockedInStaging(t *testing.T) {
	httpCfg := DefaultHTTPServerConfig()
	builder := newTestBuilder(t, "staging", httpCfg)
	builder.app.config.EnablePprof = true

	builder.registerPprofEndpoints()

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	w := httptest.NewRecorder()
	builder.mux.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Error("expected pprof endpoints to be blocked in staging even with EnablePprof=true")
	}
}

func TestRegisterPprofEndpoints_BlockedInProduction(t *testing.T) {
	httpCfg := DefaultHTTPServerConfig()
	builder := newTestBuilder(t, "production", httpCfg)
	builder.app.config.EnablePprof = true

	builder.registerPprofEndpoints()

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	w := httptest.NewRecorder()
	builder.mux.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Error("expected pprof endpoints to be blocked in production even with EnablePprof=true")
	}
}

func TestRegisterPprofEndpoints_AllowedInDevelopment(t *testing.T) {
	httpCfg := DefaultHTTPServerConfig()
	builder := newTestBuilder(t, "development", httpCfg)
	builder.app.config.EnablePprof = true

	builder.registerPprofEndpoints()

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	w := httptest.NewRecorder()
	builder.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected pprof endpoints reachable in development, got %d", w.Code)
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
