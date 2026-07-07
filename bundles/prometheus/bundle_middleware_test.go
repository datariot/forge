package prometheus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/datariot/forge/config"
	"github.com/datariot/forge/framework"
)

// newTestApp creates a minimal, un-started framework.App suitable for
// exercising Bundle.Initialize and the framework's bundle-seam methods
// (AddUnaryInterceptor/AddStreamInterceptor/AddHTTPMiddleware) without
// needing to bind real network listeners.
func newTestApp(t *testing.T) *framework.App {
	t.Helper()
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "prometheus-bundle-test"
	cfg.GRPCAddr = ":0"
	cfg.HTTPAddr = ":0"

	app, err := framework.New(framework.WithConfig(&cfg))
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}
	return app
}

// --- Initialize wiring ---

func TestBundle_Initialize_WiresAutomaticHTTPMetrics(t *testing.T) {
	reg := newTestRegistry()
	cfg := DefaultConfig()
	cfg.Namespace = "test"
	cfg.Registry = reg
	cfg.EnableGRPCMetrics = false
	b := NewBundle(cfg)
	app := newTestApp(t)

	if err := b.Initialize(app); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Build the framework's real HTTP handler the way startHTTPServer does,
	// and drive a request through it end-to-end: this is what proves the
	// metrics are actually automatic rather than requiring a manual
	// RecordHTTPRequest call in a handler.
	builder := framework.NewHTTPServerBuilder(app, framework.DefaultHTTPServerConfig())
	httpServer, err := builder.Build()
	if err != nil {
		t.Fatalf("failed to build HTTP server: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w := httptest.NewRecorder()
	httpServer.Handler.ServeHTTP(w, req)

	if count := testutil.CollectAndCount(reg, "test_http_requests_total"); count == 0 {
		t.Error("expected http_requests_total to have recorded a series after a request through the wrapped handler")
	}
}

func TestBundle_Initialize_WiresAutomaticGRPCMetrics(t *testing.T) {
	reg := newTestRegistry()
	cfg := DefaultConfig()
	cfg.Namespace = "test"
	cfg.Registry = reg
	cfg.EnableHTTPMetrics = false
	b := NewBundle(cfg)
	app := newTestApp(t)

	if err := b.Initialize(app); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Invoke the interceptor directly with a fake handler + UnaryServerInfo,
	// mirroring how grpc.ChainUnaryInterceptor would call it for a real RPC.
	interceptor := b.UnaryServerInterceptor()
	fakeHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	}
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	if _, err := interceptor(context.Background(), nil, info, fakeHandler); err != nil {
		t.Fatalf("interceptor returned unexpected error: %v", err)
	}

	if count := testutil.CollectAndCount(reg, "test_grpc_requests_total"); count == 0 {
		t.Error("expected grpc_requests_total to have recorded a series after invoking the interceptor")
	}
}

// --- UnaryServerInterceptor ---

func TestBundle_UnaryServerInterceptor_RecordsOKStatus(t *testing.T) {
	cfg := Config{
		Namespace:         "test",
		EnableGRPCMetrics: true,
		HistogramBuckets:  []float64{0.01, 0.1, 1.0},
		ServiceLabels:     map[string]string{},
	}
	b := newInitializedBundle(t, cfg)

	interceptor := b.UnaryServerInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Get"}

	_, err := interceptor(context.Background(), nil, info,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return nil, nil
		})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := testutil.ToFloat64(b.grpcRequestsTotal.WithLabelValues("/test.Service/Get", codes.OK.String())); got != 1 {
		t.Errorf("expected 1 recorded OK request, got %v", got)
	}
}

func TestBundle_UnaryServerInterceptor_RecordsErrorStatus(t *testing.T) {
	cfg := Config{
		Namespace:         "test",
		EnableGRPCMetrics: true,
		HistogramBuckets:  []float64{0.01, 0.1, 1.0},
		ServiceLabels:     map[string]string{},
	}
	b := newInitializedBundle(t, cfg)

	interceptor := b.UnaryServerInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Get"}

	_, err := interceptor(context.Background(), nil, info,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return nil, status.Error(codes.NotFound, "missing")
		})
	if err == nil {
		t.Fatal("expected handler error to propagate")
	}

	if got := testutil.ToFloat64(b.grpcRequestsTotal.WithLabelValues("/test.Service/Get", codes.NotFound.String())); got != 1 {
		t.Errorf("expected 1 recorded NotFound request, got %v", got)
	}
}

// --- StreamServerInterceptor ---

func TestBundle_StreamServerInterceptor_RecordsMetrics(t *testing.T) {
	cfg := Config{
		Namespace:         "test",
		EnableGRPCMetrics: true,
		HistogramBuckets:  []float64{0.01, 0.1, 1.0},
		ServiceLabels:     map[string]string{},
	}
	b := newInitializedBundle(t, cfg)

	interceptor := b.StreamServerInterceptor()
	info := &grpc.StreamServerInfo{FullMethod: "/test.Service/Stream"}

	err := interceptor(nil, nil, info, func(srv interface{}, stream grpc.ServerStream) error {
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := testutil.ToFloat64(b.grpcRequestsTotal.WithLabelValues("/test.Service/Stream", codes.OK.String())); got != 1 {
		t.Errorf("expected 1 recorded stream request, got %v", got)
	}
}

// --- HTTPMiddleware ---

func TestBundle_HTTPMiddleware_UsesRoutePatternWhenAvailable(t *testing.T) {
	cfg := Config{
		Namespace:         "test",
		EnableHTTPMetrics: true,
		HistogramBuckets:  []float64{0.01, 0.1, 1.0},
		ServiceLabels:     map[string]string{},
	}
	b := newInitializedBundle(t, cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := b.HTTPMiddleware()(mux)

	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := testutil.ToFloat64(b.httpRequestsTotal.WithLabelValues("GET", "GET /users/{id}", "200")); got != 1 {
		t.Errorf("expected the low-cardinality route pattern to be used as the endpoint label, got %v", got)
	}
}

func TestBundle_HTTPMiddleware_FallsBackToPathWhenUnmatched(t *testing.T) {
	cfg := Config{
		Namespace:         "test",
		EnableHTTPMetrics: true,
		HistogramBuckets:  []float64{0.01, 0.1, 1.0},
		ServiceLabels:     map[string]string{},
	}
	b := newInitializedBundle(t, cfg)

	mux := http.NewServeMux() // no routes registered, so every request 404s with no pattern
	handler := b.HTTPMiddleware()(mux)

	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := testutil.ToFloat64(b.httpRequestsTotal.WithLabelValues("GET", "/nope", "404")); got != 1 {
		t.Errorf("expected raw path fallback when no route pattern matched, got %v", got)
	}
}

func TestBundle_HTTPMiddleware_MetricsDisabled(t *testing.T) {
	cfg := Config{
		Namespace:         "test",
		EnableHTTPMetrics: false,
		HistogramBuckets:  []float64{0.01, 0.1, 1.0},
		ServiceLabels:     map[string]string{},
	}
	b := newInitializedBundle(t, cfg)

	handler := b.HTTPMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	w := httptest.NewRecorder()

	// Should not panic even though httpRequestsTotal/httpRequestDuration are nil.
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestBundle_HTTPMiddleware_ImplicitWriteRecordsOK(t *testing.T) {
	cfg := Config{
		Namespace:         "test",
		EnableHTTPMetrics: true,
		HistogramBuckets:  []float64{0.01, 0.1, 1.0},
		ServiceLabels:     map[string]string{},
	}
	b := newInitializedBundle(t, cfg)

	// Handler writes a body without ever calling WriteHeader explicitly.
	handler := b.HTTPMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/implicit", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := testutil.ToFloat64(b.httpRequestsTotal.WithLabelValues("GET", "/implicit", "200")); got != 1 {
		t.Errorf("expected implicit 200 to be recorded, got %v", got)
	}
}
