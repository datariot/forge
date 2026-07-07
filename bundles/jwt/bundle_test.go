package jwt

import (
	"context"
	"net/http"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/datariot/forge/errors"
)

// TestDefaultConfig tests default configuration values
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Note: Issuer, Audience, ServiceName, SecretKey are required to be set by user
	// DefaultConfig only sets the timing and security defaults

	if cfg.TokenDuration != 1*time.Hour {
		t.Errorf("Expected TokenDuration 1h, got %v", cfg.TokenDuration)
	}

	if cfg.ClockSkew != 1*time.Minute {
		t.Errorf("Expected ClockSkew 1m, got %v", cfg.ClockSkew)
	}

	if !cfg.RequireHTTPS {
		t.Error("Expected RequireHTTPS to be true by default (secure by default)")
	}

	if len(cfg.SkipPaths) != 1 || cfg.SkipPaths[0] != "/health" {
		t.Error("Expected SkipPaths to contain only /health by default")
	}
}

// TestNewBundle tests bundle creation
func TestNewBundle(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = []byte("test-secret-key-32-bytes-long!!")
	cfg.ServiceName = "test-service"

	bundle := NewBundle(cfg)

	if bundle == nil {
		t.Fatal("Expected bundle to be created")
	}

	if string(bundle.config.SecretKey) != string(cfg.SecretKey) {
		t.Error("Expected config to be set")
	}
}

// TestBundle_Name tests bundle name
func TestBundle_Name(t *testing.T) {
	bundle := NewBundle(DefaultConfig())

	if bundle.Name() != "jwt-auth" {
		t.Errorf("Expected bundle name 'jwt-auth', got %s", bundle.Name())
	}
}

// TestConfig_Validate_MissingSecretKey tests validation fails without secret key
func TestConfig_Validate_MissingSecretKey(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = nil

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Expected error for missing secret key")
	}

	if err.Error() != "jwt secret key is required" {
		t.Errorf("Expected 'jwt secret key is required', got: %v", err)
	}
}

// TestConfig_Validate_ShortSecretKey tests validation fails with short key
func TestConfig_Validate_ShortSecretKey(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = []byte("short")

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Expected error for short secret key")
	}
}

// TestConfig_Validate_MissingIssuer tests validation fails without issuer
func TestConfig_Validate_MissingIssuer(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = []byte("test-secret-key-32-bytes-long!!")
	cfg.Issuer = ""

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Expected error for missing issuer")
	}
}

// TestConfig_Validate_MissingAudience tests validation fails without audience
func TestConfig_Validate_MissingAudience(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = []byte("test-secret-key-32-bytes-long!!")
	cfg.Audience = ""

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Expected error for missing audience")
	}
}

// TestConfig_Validate_MissingServiceName tests validation fails without service name
func TestConfig_Validate_MissingServiceName(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = []byte("test-secret-key-32-bytes-long!!")
	cfg.ServiceName = ""

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Expected error for missing service name")
	}
}

// TestConfig_Validate_InvalidTokenDuration tests validation fails with invalid duration
func TestConfig_Validate_InvalidTokenDuration(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = []byte("test-secret-key-32-bytes-long!!")
	cfg.ServiceName = "test"
	cfg.TokenDuration = -1 * time.Second

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Expected error for negative token duration")
	}
}

// TestConfig_Validate_InvalidClockSkew tests validation fails with negative clock skew
func TestConfig_Validate_InvalidClockSkew(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = []byte("test-secret-key-32-bytes-long!!")
	cfg.ServiceName = "test"
	cfg.ClockSkew = -1 * time.Second

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Expected error for negative clock skew")
	}
}

// TestConfig_Validate_Success tests successful validation
func TestConfig_Validate_Success(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = []byte("this-is-a-32-byte-secret-key!!!!") // Exactly 32 bytes
	cfg.ServiceName = "test-service"
	cfg.Issuer = "test-issuer"
	cfg.Audience = "test-audience"

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Expected validation to pass, got: %v", err)
	}
}

// TestBundle_Initialize_InvalidConfig tests initialization fails with invalid config
func TestBundle_Initialize_InvalidConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = nil // Invalid

	bundle := NewBundle(cfg)
	err := bundle.Initialize(nil)

	if err == nil {
		t.Fatal("Expected error for invalid configuration")
	}

	if !errors.IsConfigurationError(err) {
		t.Error("Expected configuration error")
	}
}

// TestBundle_Stop tests Stop method
func TestBundle_Stop(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = []byte("test-secret-key-32-bytes-long!!")
	cfg.ServiceName = "test-service"

	bundle := NewBundle(cfg)

	ctx := context.Background()
	if err := bundle.Stop(ctx); err != nil {
		t.Errorf("Expected Stop to succeed, got: %v", err)
	}
}

// newValidBundle creates a bundle with a valid config for testing.
func newValidBundle() *Bundle {
	cfg := DefaultConfig()
	cfg.SecretKey = []byte("this-is-a-32-byte-secret-key!!!!")
	cfg.ServiceName = "test-service"
	cfg.Issuer = "test-issuer"
	cfg.Audience = "test-audience"
	cfg.RequireHTTPS = false // disable for tests
	return NewBundle(cfg)
}

// TestBundle_GenerateToken_Valid tests successful token generation.
func TestBundle_GenerateToken_Valid(t *testing.T) {
	b := newValidBundle()

	token, err := b.GenerateToken("svc-123", "test-service", []string{"read"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token")
	}
}

// TestBundle_GenerateToken_InvalidServiceID rejects invalid service IDs.
func TestBundle_GenerateToken_InvalidServiceID(t *testing.T) {
	b := newValidBundle()
	_, err := b.GenerateToken("invalid service!", "test-service", nil)
	if err == nil {
		t.Error("expected error for invalid service ID")
	}
}

// TestBundle_GenerateToken_InvalidServiceName rejects invalid service names.
func TestBundle_GenerateToken_InvalidServiceName(t *testing.T) {
	b := newValidBundle()
	_, err := b.GenerateToken("valid-id", "invalid service name!", nil)
	if err == nil {
		t.Error("expected error for invalid service name")
	}
}

// TestBundle_ValidateToken_Valid validates a freshly generated token.
func TestBundle_ValidateToken_Valid(t *testing.T) {
	b := newValidBundle()

	token, err := b.GenerateToken("svc-123", "test-service", []string{"read"})
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	claims, err := b.ValidateToken(token)
	if err != nil {
		t.Fatalf("expected valid token, got error: %v", err)
	}
	if claims.ServiceID != "svc-123" {
		t.Errorf("expected ServiceID='svc-123', got %q", claims.ServiceID)
	}
	if claims.ServiceName != "test-service" {
		t.Errorf("expected ServiceName='test-service', got %q", claims.ServiceName)
	}
}

// TestBundle_ValidateToken_EmptyToken returns error for empty token.
func TestBundle_ValidateToken_EmptyToken(t *testing.T) {
	b := newValidBundle()
	_, err := b.ValidateToken("")
	if err == nil {
		t.Error("expected error for empty token")
	}
}

// TestBundle_ValidateToken_InvalidToken returns error for garbage.
func TestBundle_ValidateToken_InvalidToken(t *testing.T) {
	b := newValidBundle()
	_, err := b.ValidateToken("not.a.valid.jwt.token")
	if err == nil {
		t.Error("expected error for invalid token")
	}
}

// TestBundle_ValidateToken_WrongSecret fails with different key.
func TestBundle_ValidateToken_WrongSecret(t *testing.T) {
	b := newValidBundle()
	token, err := b.GenerateToken("svc-123", "test-service", nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	// Use different secret
	cfg := DefaultConfig()
	cfg.SecretKey = []byte("different-secret-key-32bytes!!!!")
	cfg.ServiceName = "test-service"
	cfg.Issuer = "test-issuer"
	cfg.Audience = "test-audience"
	cfg.RequireHTTPS = false
	b2 := NewBundle(cfg)

	_, err = b2.ValidateToken(token)
	if err == nil {
		t.Error("expected error for token with wrong secret")
	}
}

// TestBundle_HTTPMiddleware_SkipsHealthPath tests that health path bypasses auth.
func TestBundle_HTTPMiddleware_SkipsHealthPath(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = []byte("this-is-a-32-byte-secret-key!!!!")
	cfg.ServiceName = "test-service"
	cfg.Issuer = "test-issuer"
	cfg.Audience = "test-audience"
	cfg.RequireHTTPS = false
	// SkipPaths uses path without leading slash after cleaning
	cfg.SkipPaths = []string{"health"}
	b := NewBundle(cfg)

	handlerCalled := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := b.HTTPMiddleware(inner)

	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	w := newResponseRecorder()
	handler.ServeHTTP(w, req)

	if !handlerCalled {
		t.Error("expected handler to be called for /health path")
	}
}

// TestBundle_HTTPMiddleware_RejectsUnauthenticated returns 401 without token.
func TestBundle_HTTPMiddleware_RejectsUnauthenticated(t *testing.T) {
	b := newValidBundle()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := b.HTTPMiddleware(inner)

	req, _ := http.NewRequest(http.MethodGet, "/api/data", nil)
	w := newResponseRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestBundle_HTTPMiddleware_ValidToken allows authenticated request.
func TestBundle_HTTPMiddleware_ValidToken(t *testing.T) {
	b := newValidBundle()

	token, err := b.GenerateToken("svc-123", "test-service", nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	handlerCalled := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})
	handler := b.HTTPMiddleware(inner)

	req, _ := http.NewRequest(http.MethodGet, "/api/data", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := newResponseRecorder()
	handler.ServeHTTP(w, req)

	if !handlerCalled {
		t.Error("expected handler to be called with valid token")
	}
}

// TestBundle_ClaimsFromContext tests round-trip of claims in context.
func TestBundle_ClaimsFromContext(t *testing.T) {
	b := newValidBundle()

	token, err := b.GenerateToken("svc-abc", "my-service", []string{"admin"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	claims, err := b.ValidateToken(token)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	ctx := b.contextWithClaims(context.Background(), claims)
	extracted := ClaimsFromContext(ctx)
	if extracted == nil {
		t.Fatal("expected claims in context, got nil")
	}
	if extracted.ServiceID != "svc-abc" {
		t.Errorf("expected ServiceID='svc-abc', got %q", extracted.ServiceID)
	}
}

// responseRecorder is a minimal ResponseWriter for JWT tests.
type responseRecorder struct {
	Code   int
	header http.Header
}

func newResponseRecorder() *responseRecorder {
	return &responseRecorder{Code: 200, header: make(http.Header)}
}

func (r *responseRecorder) Header() http.Header         { return r.header }
func (r *responseRecorder) Write(b []byte) (int, error) { return len(b), nil }
func (r *responseRecorder) WriteHeader(code int)        { r.Code = code }

// TestHasPermission tests permission checking.
func TestHasPermission(t *testing.T) {
	b := newValidBundle()
	token, _ := b.GenerateToken("svc-123", "test-service", []string{"read", "write"})
	claims, _ := b.ValidateToken(token)
	ctx := b.contextWithClaims(context.Background(), claims)

	if !HasPermission(ctx, "read") {
		t.Error("expected HasPermission=true for 'read'")
	}
	if !HasPermission(ctx, "write") {
		t.Error("expected HasPermission=true for 'write'")
	}
	if HasPermission(ctx, "admin") {
		t.Error("expected HasPermission=false for 'admin'")
	}
}

// TestHasPermission_NoClaims returns false when no claims in context.
func TestHasPermission_NoClaims(t *testing.T) {
	if HasPermission(context.Background(), "read") {
		t.Error("expected false without claims in context")
	}
}

// TestGetServiceID tests service ID extraction from context.
func TestGetServiceID(t *testing.T) {
	b := newValidBundle()
	token, _ := b.GenerateToken("my-svc", "test-service", nil)
	claims, _ := b.ValidateToken(token)
	ctx := b.contextWithClaims(context.Background(), claims)

	if id := GetServiceID(ctx); id != "my-svc" {
		t.Errorf("expected 'my-svc', got %q", id)
	}
}

// TestGetServiceID_NoClaims returns empty string without claims.
func TestGetServiceID_NoClaims(t *testing.T) {
	if id := GetServiceID(context.Background()); id != "" {
		t.Errorf("expected empty string, got %q", id)
	}
}

// TestGetServiceName tests service name extraction from context.
func TestGetServiceName(t *testing.T) {
	b := newValidBundle()
	token, _ := b.GenerateToken("svc-123", "my-service", nil)
	claims, _ := b.ValidateToken(token)
	ctx := b.contextWithClaims(context.Background(), claims)

	if name := GetServiceName(ctx); name != "my-service" {
		t.Errorf("expected 'my-service', got %q", name)
	}
}

// TestGetServiceName_NoClaims returns empty string without claims.
func TestGetServiceName_NoClaims(t *testing.T) {
	if name := GetServiceName(context.Background()); name != "" {
		t.Errorf("expected empty string, got %q", name)
	}
}

// TestBundle_GenerateServiceToken tests service token generation.
func TestBundle_GenerateServiceToken(t *testing.T) {
	b := newValidBundle()
	token, err := b.GenerateServiceToken([]string{"admin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token")
	}
}

// TestBundle_RequirePermissions_WithPermission allows request with correct permission.
func TestBundle_RequirePermissions_WithPermission(t *testing.T) {
	b := newValidBundle()
	token, _ := b.GenerateToken("svc-123", "test-service", []string{"read"})
	claims, _ := b.ValidateToken(token)

	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	middleware := b.RequirePermissions("read")
	handler := middleware(inner)

	req, _ := http.NewRequest(http.MethodGet, "/api", nil)
	ctx := b.contextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)
	w := newResponseRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("expected handler to be called with correct permission")
	}
}

// TestBundle_RequirePermissions_MissingPermission returns 403.
func TestBundle_RequirePermissions_MissingPermission(t *testing.T) {
	b := newValidBundle()
	token, _ := b.GenerateToken("svc-123", "test-service", []string{"read"})
	claims, _ := b.ValidateToken(token)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	middleware := b.RequirePermissions("admin")
	handler := middleware(inner)

	req, _ := http.NewRequest(http.MethodGet, "/api", nil)
	ctx := b.contextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)
	w := newResponseRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// TestBundle_RequirePermissions_NoClaims returns 401.
func TestBundle_RequirePermissions_NoClaims(t *testing.T) {
	b := newValidBundle()
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	middleware := b.RequirePermissions("read")
	handler := middleware(inner)

	req, _ := http.NewRequest(http.MethodGet, "/api", nil)
	w := newResponseRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// Note: Service identifier format validation (alphanumeric check) happens during
// token generation/validation, not during config validation. Config validation only
// checks that required fields are present.

// TestServiceClaims_Validation tests claims structure
func TestServiceClaims_Validation(t *testing.T) {
	claims := &ServiceClaims{
		ServiceID:   "svc-123",
		ServiceName: "test-service",
		Permissions: []string{"read", "write"},
	}

	if claims.ServiceID != "svc-123" {
		t.Error("ServiceID not set correctly")
	}

	if claims.ServiceName != "test-service" {
		t.Error("ServiceName not set correctly")
	}

	if len(claims.Permissions) != 2 {
		t.Error("Permissions not set correctly")
	}
}

func TestShouldSkipPath_PrefixWildcard(t *testing.T) {
	b := newValidBundle()
	b.config.SkipPaths = []string{"api/*"}

	if !b.shouldSkipPath("api/v1/resource") {
		t.Error("expected api/v1/resource to match prefix wildcard api/*")
	}
	if b.shouldSkipPath("other/path") {
		t.Error("expected other/path to not match api/*")
	}
}

func TestShouldSkipPath_DirectoryPrefix(t *testing.T) {
	b := newValidBundle()
	b.config.SkipPaths = []string{"public/"}

	if !b.shouldSkipPath("public/assets/logo.png") {
		t.Error("expected public/assets/logo.png to match public/ prefix")
	}
	if b.shouldSkipPath("private/data") {
		t.Error("expected private/data to not match public/ prefix")
	}
}

func TestShouldSkipPath_RootPath(t *testing.T) {
	b := newValidBundle()
	b.config.SkipPaths = []string{"health"}

	if !b.shouldSkipPath("health") {
		t.Error("expected 'health' to match skip path 'health'")
	}
	if b.shouldSkipPath("") {
		t.Error("expected empty path to not match skip path")
	}
}

func TestValidateServiceIdentifier_Valid(t *testing.T) {
	if err := validateServiceIdentifier("my-service"); err != nil {
		t.Errorf("expected valid identifier, got: %v", err)
	}
	if err := validateServiceIdentifier("svc_123"); err != nil {
		t.Errorf("expected valid identifier, got: %v", err)
	}
}

func TestValidateServiceIdentifier_Empty(t *testing.T) {
	if err := validateServiceIdentifier(""); err == nil {
		t.Error("expected error for empty identifier")
	}
}

func TestValidateServiceIdentifier_TooShort(t *testing.T) {
	if err := validateServiceIdentifier("a"); err == nil {
		t.Error("expected error for single-char identifier")
	}
}

func TestValidateServiceIdentifier_TooLong(t *testing.T) {
	long := "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnop" // >64
	if err := validateServiceIdentifier(long); err == nil {
		t.Error("expected error for too-long identifier")
	}
}

func TestValidateServiceIdentifier_InvalidChars(t *testing.T) {
	if err := validateServiceIdentifier("svc@invalid!"); err == nil {
		t.Error("expected error for identifier with invalid characters")
	}
}

// fakeServerStream is a minimal grpc.ServerStream whose Context() is
// controllable, used to drive StreamServerInterceptor in tests.
type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeServerStream) Context() context.Context { return f.ctx }

// TestBundle_StreamServerInterceptor_RejectsMissingToken returns
// Unauthenticated when the stream context carries no metadata.
func TestBundle_StreamServerInterceptor_RejectsMissingToken(t *testing.T) {
	b := newValidBundle()
	interceptor := b.StreamServerInterceptor()

	stream := &fakeServerStream{ctx: context.Background()}
	handlerCalled := false
	handler := func(srv interface{}, ss grpc.ServerStream) error {
		handlerCalled = true
		return nil
	}

	err := interceptor(nil, stream, &grpc.StreamServerInfo{}, handler)
	if err == nil {
		t.Fatal("expected error for missing token")
	}
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", status.Code(err))
	}
	if handlerCalled {
		t.Error("expected handler not to be called")
	}
}

// TestBundle_StreamServerInterceptor_RejectsInvalidToken returns
// Unauthenticated when the token fails validation.
func TestBundle_StreamServerInterceptor_RejectsInvalidToken(t *testing.T) {
	b := newValidBundle()
	interceptor := b.StreamServerInterceptor()

	md := metadata.Pairs("authorization", "Bearer not-a-real-token")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	stream := &fakeServerStream{ctx: ctx}
	handlerCalled := false
	handler := func(srv interface{}, ss grpc.ServerStream) error {
		handlerCalled = true
		return nil
	}

	err := interceptor(nil, stream, &grpc.StreamServerInfo{}, handler)
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", status.Code(err))
	}
	if handlerCalled {
		t.Error("expected handler not to be called")
	}
}

// TestBundle_StreamServerInterceptor_AcceptsValidToken calls the handler with
// a wrapped stream whose Context() carries the authenticated claims.
func TestBundle_StreamServerInterceptor_AcceptsValidToken(t *testing.T) {
	b := newValidBundle()
	interceptor := b.StreamServerInterceptor()

	token, err := b.GenerateToken("svc-123", "test-service", []string{"read"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	md := metadata.Pairs("authorization", "Bearer "+token)
	ctx := metadata.NewIncomingContext(context.Background(), md)
	stream := &fakeServerStream{ctx: ctx}

	var gotClaims *ServiceClaims
	handler := func(srv interface{}, ss grpc.ServerStream) error {
		gotClaims = ClaimsFromContext(ss.Context())
		return nil
	}

	if err := interceptor(nil, stream, &grpc.StreamServerInfo{}, handler); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if gotClaims == nil {
		t.Fatal("expected claims to be present in wrapped stream context")
	}
	if gotClaims.ServiceID != "svc-123" {
		t.Errorf("expected ServiceID='svc-123', got %q", gotClaims.ServiceID)
	}
}

// TestBundle_RequireHTTPS_UntrustedProxyHeader_ForgedHeaderIgnored verifies
// that a forged X-Forwarded-Proto header does not satisfy RequireHTTPS when
// TrustedProxyHeader is left at its default (false).
func TestBundle_RequireHTTPS_UntrustedProxyHeader_ForgedHeaderIgnored(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = []byte("this-is-a-32-byte-secret-key!!!!")
	cfg.ServiceName = "test-service"
	cfg.Issuer = "test-issuer"
	cfg.Audience = "test-audience"
	cfg.RequireHTTPS = true
	// TrustedProxyHeader defaults to false.
	b := NewBundle(cfg)

	handlerCalled := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})
	handler := b.HTTPMiddleware(inner)

	req, _ := http.NewRequest(http.MethodGet, "/api/data", nil)
	req.Header.Set("X-Forwarded-Proto", "https") // forged by an untrusted client
	w := newResponseRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 (HTTPS required), got %d", w.Code)
	}
	if handlerCalled {
		t.Error("expected handler not to be called when forged header is ignored")
	}
}

// TestBundle_RequireHTTPS_TrustedProxyHeader_HonorsForwardedProto verifies
// that setting TrustedProxyHeader=true honors X-Forwarded-Proto, preserving
// existing behavior for operators behind a trusted terminating proxy.
func TestBundle_RequireHTTPS_TrustedProxyHeader_HonorsForwardedProto(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = []byte("this-is-a-32-byte-secret-key!!!!")
	cfg.ServiceName = "test-service"
	cfg.Issuer = "test-issuer"
	cfg.Audience = "test-audience"
	cfg.RequireHTTPS = true
	cfg.TrustedProxyHeader = true
	b := NewBundle(cfg)

	token, err := b.GenerateToken("svc-123", "test-service", nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	handlerCalled := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})
	handler := b.HTTPMiddleware(inner)

	req, _ := http.NewRequest(http.MethodGet, "/api/data", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Authorization", "Bearer "+token)
	w := newResponseRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !handlerCalled {
		t.Error("expected handler to be called when trusted proxy header indicates https")
	}
}

// TestBundle_RequireHTTPS_TrustedProxyHeader_StillRejectsPlainHTTP verifies
// that enabling TrustedProxyHeader doesn't disable enforcement outright.
func TestBundle_RequireHTTPS_TrustedProxyHeader_StillRejectsPlainHTTP(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = []byte("this-is-a-32-byte-secret-key!!!!")
	cfg.ServiceName = "test-service"
	cfg.Issuer = "test-issuer"
	cfg.Audience = "test-audience"
	cfg.RequireHTTPS = true
	cfg.TrustedProxyHeader = true
	b := NewBundle(cfg)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := b.HTTPMiddleware(inner)

	req, _ := http.NewRequest(http.MethodGet, "/api/data", nil)
	// No X-Forwarded-Proto and no TLS.
	w := newResponseRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 (HTTPS required), got %d", w.Code)
	}
}
