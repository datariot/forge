package framework

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/datariot/forge/config"
)

// --- AddUnaryInterceptor / AddStreamInterceptor ---

func TestApp_AddUnaryInterceptor_Nil(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "add-unary-interceptor-nil-test"

	app, err := New(WithConfig(&cfg))
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	app.AddUnaryInterceptor(nil)
	if len(app.unaryInterceptors) != 0 {
		t.Errorf("expected nil interceptor to be ignored, got %d registered", len(app.unaryInterceptors))
	}
}

func TestApp_AddStreamInterceptor_Nil(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "add-stream-interceptor-nil-test"

	app, err := New(WithConfig(&cfg))
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	app.AddStreamInterceptor(nil)
	if len(app.streamInterceptors) != 0 {
		t.Errorf("expected nil interceptor to be ignored, got %d registered", len(app.streamInterceptors))
	}
}

// TestApp_AddUnaryInterceptor_RunsForRealCall verifies that a unary
// interceptor registered via the bundle seam (AddUnaryInterceptor, called
// the way a bundle would from Initialize) actually runs for a real gRPC
// call, using the always-registered health service as the callee.
func TestApp_AddUnaryInterceptor_RunsForRealCall(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "add-unary-interceptor-test"
	cfg.GRPCAddr = ":0"
	cfg.HTTPAddr = ":0"

	app, err := New(WithConfig(&cfg), WithGRPCRegistrar(&testRegistrar{}))
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	called := false
	app.AddUnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		called = true
		return handler(ctx, req)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := app.Start(ctx); err != nil {
		t.Fatalf("failed to start app: %v", err)
	}
	defer func() { _ = app.Stop(context.Background()) }()

	conn, err := grpc.NewClient(app.grpcListener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to dial grpc server: %v", err)
	}
	defer func() { _ = conn.Close() }()

	client := grpc_health_v1.NewHealthClient(conn)
	if _, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{}); err != nil {
		t.Fatalf("health check RPC failed: %v", err)
	}

	if !called {
		t.Error("expected unary interceptor registered via AddUnaryInterceptor to run")
	}
}

// TestApp_AddUnaryInterceptor_NoopAfterServerBuilt verifies that adding an
// interceptor after the gRPC server has already been constructed is a
// documented no-op rather than a panic or a silently-ignored-but-misleading
// success.
func TestApp_AddUnaryInterceptor_NoopAfterServerBuilt(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "add-unary-interceptor-noop-test"
	cfg.GRPCAddr = ":0"
	cfg.HTTPAddr = ":0"

	app, err := New(WithConfig(&cfg), WithGRPCRegistrar(&testRegistrar{}))
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := app.Start(ctx); err != nil {
		t.Fatalf("failed to start app: %v", err)
	}
	defer func() { _ = app.Stop(context.Background()) }()

	before := len(app.unaryInterceptors)
	app.AddUnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return handler(ctx, req)
	})
	if len(app.unaryInterceptors) != before {
		t.Error("expected AddUnaryInterceptor to no-op once the gRPC server is built")
	}
}

// --- AddHTTPMiddleware ---

func TestApp_AddHTTPMiddleware_Nil(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "add-http-middleware-nil-test"

	app, err := New(WithConfig(&cfg))
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	app.AddHTTPMiddleware(nil)
	if len(app.httpMiddlewares) != 0 {
		t.Errorf("expected nil middleware to be ignored, got %d registered", len(app.httpMiddlewares))
	}
}

// TestApp_AddHTTPMiddleware_RunsForHealthEndpoint verifies that HTTP
// middleware registered via the bundle seam (AddHTTPMiddleware, called the
// way a bundle would from Initialize) actually wraps the handler the
// framework builds, including the built-in /health route.
func TestApp_AddHTTPMiddleware_RunsForHealthEndpoint(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "add-http-middleware-test"
	cfg.GRPCAddr = ":0"
	cfg.HTTPAddr = ":0"

	app, err := New(WithConfig(&cfg))
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	called := false
	app.AddHTTPMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			next.ServeHTTP(w, r)
		})
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := app.Start(ctx); err != nil {
		t.Fatalf("failed to start app: %v", err)
	}
	defer func() { _ = app.Stop(context.Background()) }()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	app.httpServer.Handler.ServeHTTP(w, req)

	if !called {
		t.Error("expected HTTP middleware registered via AddHTTPMiddleware to run for /health")
	}
	if w.Code != http.StatusOK && w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected /health to still respond normally through the wrapped handler, got %d", w.Code)
	}
}

// TestApp_AddHTTPMiddleware_NoopAfterServerBuilt verifies that adding
// middleware after the HTTP server has already been built is a documented
// no-op.
func TestApp_AddHTTPMiddleware_NoopAfterServerBuilt(t *testing.T) {
	cfg := config.DefaultBaseConfig()
	cfg.ServiceName = "add-http-middleware-noop-test"
	cfg.GRPCAddr = ":0"
	cfg.HTTPAddr = ":0"

	app, err := New(WithConfig(&cfg))
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := app.Start(ctx); err != nil {
		t.Fatalf("failed to start app: %v", err)
	}
	defer func() { _ = app.Stop(context.Background()) }()

	before := len(app.httpMiddlewares)
	app.AddHTTPMiddleware(func(next http.Handler) http.Handler { return next })
	if len(app.httpMiddlewares) != before {
		t.Error("expected AddHTTPMiddleware to no-op once the HTTP server is built")
	}
}
