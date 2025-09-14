// Package jwt provides JWT authentication bundle for secure inter-service communication.
//
// The JWT bundle provides:
//   - JWT token validation and signing
//   - gRPC authentication interceptors for both client and server
//   - HTTP authentication middleware
//   - Token propagation between services
//   - Service identity management
//   - Configurable token validation (expiry, audience, issuer)
//
// # Basic Usage
//
// Configure JWT authentication for your service:
//
//	jwtConfig := jwt.Config{
//		SecretKey:     []byte("your-secret-key"),
//		Issuer:        "auth-service",
//		Audience:      "forge-services",
//		TokenDuration: 24 * time.Hour,
//		ServiceName:   "user-service",
//	}
//
//	jwtBundle := jwt.NewBundle(jwtConfig)
//
//	app, err := framework.New(
//		framework.WithConfig(&baseConfig),
//		framework.WithBundle(jwtBundle),
//	)
//
// # Service-to-Service Authentication
//
// The bundle automatically adds authentication to gRPC calls:
//
//	// Client side (automatic token injection)
//	conn, err := grpc.Dial("target-service:8080",
//		grpc.WithUnaryInterceptor(jwtBundle.UnaryClientInterceptor()),
//	)
//
//	// Server side (automatic token validation)
//	app, err := framework.New(
//		framework.WithGRPCInterceptor(jwtBundle.UnaryServerInterceptor()),
//	)
//
// # HTTP Authentication
//
// Secure HTTP endpoints with JWT middleware:
//
//	// Protect specific endpoints
//	protectedMux := http.NewServeMux()
//	protectedMux.HandleFunc("/api/users", userHandler)
//
//	// Apply JWT middleware
//	http.Handle("/api/", jwtBundle.HTTPMiddleware(protectedMux))
//
// # Token Claims
//
// Access authenticated service information in handlers:
//
//	func userHandler(ctx context.Context, req *UserRequest) (*UserResponse, error) {
//		claims := jwt.ClaimsFromContext(ctx)
//		if claims == nil {
//			return nil, status.Errorf(codes.Unauthenticated, "no authentication")
//		}
//
//		serviceID := claims.ServiceID
//		// ... use service identity for authorization
//	}
package jwt

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/datariot/forge/errors"
	"github.com/datariot/forge/framework"
)

// Config contains JWT authentication configuration.
type Config struct {
	// SecretKey is the HMAC secret key for signing and validating tokens.
	// This should be a strong, random key shared across all services.
	SecretKey []byte

	// Issuer is the JWT issuer claim (typically your auth service).
	Issuer string

	// Audience is the JWT audience claim (typically your service mesh or organization).
	Audience string

	// TokenDuration is how long tokens remain valid.
	TokenDuration time.Duration

	// ServiceName is the name of this service (used in service-to-service tokens).
	ServiceName string

	// ClockSkew is the allowed time difference when validating token timestamps.
	ClockSkew time.Duration

	// RequireHTTPS enforces HTTPS for HTTP authentication (recommended for production).
	RequireHTTPS bool

	// SkipPaths are HTTP paths that don't require authentication.
	SkipPaths []string
}

// DefaultConfig returns a Config with sensible secure defaults.
func DefaultConfig() Config {
	return Config{
		TokenDuration: 1 * time.Hour,  // Shorter duration for better security
		ClockSkew:     1 * time.Minute, // Reduced clock skew tolerance
		RequireHTTPS:  true,            // Secure by default
		SkipPaths:     []string{"/health"}, // Only health checks skip auth by default
	}
}

// Validate validates the JWT configuration.
func (c *Config) Validate() error {
	if len(c.SecretKey) == 0 {
		return fmt.Errorf("jwt secret key is required")
	}

	if len(c.SecretKey) < 32 {
		return fmt.Errorf("jwt secret key must be at least 32 bytes for security")
	}

	if c.Issuer == "" {
		return fmt.Errorf("jwt issuer is required")
	}

	if c.Audience == "" {
		return fmt.Errorf("jwt audience is required")
	}

	if c.ServiceName == "" {
		return fmt.Errorf("service name is required for JWT authentication")
	}

	if c.TokenDuration <= 0 {
		return fmt.Errorf("token duration must be positive")
	}

	if c.ClockSkew < 0 {
		return fmt.Errorf("clock skew must be non-negative")
	}

	return nil
}

// ServiceClaims represents the claims in a service-to-service JWT token.
type ServiceClaims struct {
	jwt.RegisteredClaims
	ServiceID   string   `json:"service_id"`
	ServiceName string   `json:"service_name"`
	Permissions []string `json:"permissions,omitempty"`
}

// Bundle provides JWT authentication for Forge applications.
type Bundle struct {
	config      Config
	parser      *jwt.Parser
	tokenCache  *tokenCache
}

// NewBundle creates a new JWT authentication bundle.
func NewBundle(config Config) *Bundle {
	return &Bundle{
		config:     config,
		parser:     jwt.NewParser(jwt.WithValidMethods([]string{"HS256"})),
		tokenCache: newTokenCache(),
	}
}

// Name returns the bundle name.
func (b *Bundle) Name() string {
	return "jwt-auth"
}

// Initialize sets up JWT authentication.
func (b *Bundle) Initialize(app *framework.App) error {
	if err := b.config.Validate(); err != nil {
		return errors.ErrInvalidConfiguration.WithMessage("JWT configuration validation failed").WithCause(err)
	}

	return nil
}

// GenerateToken creates a new JWT token for service-to-service communication.
func (b *Bundle) GenerateToken(serviceID, serviceName string, permissions []string) (string, error) {
	// Validate service identifiers to prevent injection attacks
	if err := validateServiceIdentifier(serviceID); err != nil {
		return "", fmt.Errorf("invalid service ID: %w", err)
	}
	if err := validateServiceIdentifier(serviceName); err != nil {
		return "", fmt.Errorf("invalid service name: %w", err)
	}

	now := time.Now()

	claims := ServiceClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    b.config.Issuer,
			Audience:  jwt.ClaimStrings{b.config.Audience},
			Subject:   serviceID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(b.config.TokenDuration)),
			NotBefore: jwt.NewNumericDate(now),
		},
		ServiceID:   serviceID,
		ServiceName: serviceName,
		Permissions: permissions,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(b.config.SecretKey)
}

// ValidateToken validates a JWT token and returns the claims.
func (b *Bundle) ValidateToken(tokenString string) (*ServiceClaims, error) {
	if tokenString == "" {
		return nil, errors.ErrInvalidCredential.WithMessage("token is required")
	}

	// Parse and validate token
	token, err := b.parser.ParseWithClaims(tokenString, &ServiceClaims{}, func(token *jwt.Token) (interface{}, error) {
		return b.config.SecretKey, nil
	})

	if err != nil {
		return nil, errors.ErrInvalidCredential.WithMessage("invalid token").WithCause(err)
	}

	if !token.Valid {
		return nil, errors.ErrInvalidCredential.WithMessage("token is not valid")
	}

	claims, ok := token.Claims.(*ServiceClaims)
	if !ok {
		return nil, errors.ErrInvalidCredential.WithMessage("invalid token claims")
	}

	// Validate claims
	if err := b.validateClaims(claims); err != nil {
		return nil, err
	}

	return claims, nil
}

// validateClaims performs additional validation on token claims.
func (b *Bundle) validateClaims(claims *ServiceClaims) error {
	now := time.Now()

	// Check expiration with clock skew
	if claims.ExpiresAt != nil && now.After(claims.ExpiresAt.Time.Add(b.config.ClockSkew)) {
		return errors.ErrInvalidCredential.WithMessage("token has expired")
	}

	// Check not before with clock skew (subtract clock skew to be more permissive)
	if claims.NotBefore != nil && now.Before(claims.NotBefore.Time.Add(-b.config.ClockSkew)) {
		return errors.ErrInvalidCredential.WithMessage("token not yet valid")
	}

	// Validate audience
	if claims.Audience == nil || len(claims.Audience) == 0 {
		return errors.ErrInvalidCredential.WithMessage("token missing audience")
	}

	audienceValid := false
	for _, aud := range claims.Audience {
		if aud == b.config.Audience {
			audienceValid = true
			break
		}
	}
	if !audienceValid {
		return errors.ErrInvalidCredential.WithMessage("invalid token audience")
	}

	// Validate issuer
	if claims.Issuer != b.config.Issuer {
		return errors.ErrInvalidCredential.WithMessage("invalid token issuer")
	}

	return nil
}

// UnaryServerInterceptor returns a gRPC server interceptor for JWT authentication.
func (b *Bundle) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Extract token from gRPC metadata
		token, err := b.extractTokenFromMetadata(ctx)
		if err != nil {
			// Log detailed error for debugging, return generic error to client
			// TODO: Add proper logging when logger is available
			return nil, status.Errorf(codes.Unauthenticated, "authentication required")
		}

		// Validate token
		claims, err := b.ValidateToken(token)
		if err != nil {
			// Log detailed error for debugging, return generic error to client
			// TODO: Add proper logging when logger is available
			return nil, status.Errorf(codes.Unauthenticated, "invalid credentials")
		}

		// Add claims to context
		ctx = b.contextWithClaims(ctx, claims)

		return handler(ctx, req)
	}
}

// UnaryClientInterceptor returns a gRPC client interceptor for JWT token injection.
func (b *Bundle) UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		// Get or generate cached token for this service
		cacheKey := fmt.Sprintf("service:%s:permissions:", b.config.ServiceName)

		token, err := b.tokenCache.getOrGenerate(cacheKey, func() (string, time.Time, error) {
			token, err := b.GenerateToken(b.config.ServiceName, b.config.ServiceName, []string{})
			if err != nil {
				return "", time.Time{}, err
			}

			// Calculate expiry time (token duration minus buffer for safety)
			expiresAt := time.Now().Add(b.config.TokenDuration - 5*time.Minute)
			return token, expiresAt, nil
		})

		if err != nil {
			return fmt.Errorf("failed to get service token: %w", err)
		}

		// Add token to gRPC metadata
		ctx = b.addTokenToMetadata(ctx, token)

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// HTTPMiddleware returns HTTP middleware for JWT authentication.
func (b *Bundle) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if path should be skipped
		if b.shouldSkipPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// Enforce HTTPS in production if configured
		if b.config.RequireHTTPS && r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
			http.Error(w, "HTTPS required", http.StatusBadRequest)
			return
		}

		// Extract token from Authorization header
		token, err := b.extractTokenFromHTTP(r)
		if err != nil {
			// Log detailed error for debugging, return generic error to client
			// TODO: Add proper logging when logger is available
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		// Validate token
		claims, err := b.ValidateToken(token)
		if err != nil {
			// Log detailed error for debugging, return generic error to client
			// TODO: Add proper logging when logger is available
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}

		// Add claims to request context
		ctx := b.contextWithClaims(r.Context(), claims)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}

// extractTokenFromMetadata extracts JWT token from gRPC metadata.
func (b *Bundle) extractTokenFromMetadata(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", fmt.Errorf("no metadata found")
	}

	authHeaders := md.Get("authorization")
	if len(authHeaders) == 0 {
		return "", fmt.Errorf("no authorization header found")
	}

	authHeader := authHeaders[0]
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", fmt.Errorf("invalid authorization header format")
	}

	return strings.TrimPrefix(authHeader, "Bearer "), nil
}

// addTokenToMetadata adds JWT token to gRPC metadata for outgoing calls.
func (b *Bundle) addTokenToMetadata(ctx context.Context, token string) context.Context {
	md := metadata.Pairs("authorization", "Bearer "+token)
	return metadata.NewOutgoingContext(ctx, md)
}

// extractTokenFromHTTP extracts JWT token from HTTP Authorization header.
func (b *Bundle) extractTokenFromHTTP(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", fmt.Errorf("no authorization header found")
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", fmt.Errorf("invalid authorization header format")
	}

	return strings.TrimPrefix(authHeader, "Bearer "), nil
}

// shouldSkipPath checks if the given path should skip authentication.
// This function prevents path traversal attacks by cleaning paths and using secure matching.
func (b *Bundle) shouldSkipPath(path string) bool {
	// Clean the path to prevent traversal attacks like /health/../admin
	cleanPath := filepath.Clean("/" + path)
	if cleanPath == "/" {
		cleanPath = path // Preserve original for root path
	} else {
		cleanPath = cleanPath[1:] // Remove leading slash after cleaning
	}

	for _, skipPath := range b.config.SkipPaths {
		// Exact match
		if cleanPath == skipPath {
			return true
		}

		// Prefix match only for paths explicitly ending with /*
		if strings.HasSuffix(skipPath, "/*") {
			prefix := strings.TrimSuffix(skipPath, "/*")
			if prefix != "" && strings.HasPrefix(cleanPath, prefix+"/") {
				return true
			}
		}

		// Prefix match for directory paths ending with /
		if strings.HasSuffix(skipPath, "/") && strings.HasPrefix(cleanPath, skipPath) {
			return true
		}
	}

	return false
}

// contextWithClaims adds JWT claims to the context.
func (b *Bundle) contextWithClaims(ctx context.Context, claims *ServiceClaims) context.Context {
	return context.WithValue(ctx, claimsContextKeyType{}, claims)
}

// ClaimsFromContext extracts JWT claims from the context.
func ClaimsFromContext(ctx context.Context) *ServiceClaims {
	claims, ok := ctx.Value(claimsContextKeyType{}).(*ServiceClaims)
	if !ok {
		return nil
	}
	return claims
}

// HasPermission checks if the authenticated service has the specified permission.
func HasPermission(ctx context.Context, permission string) bool {
	claims := ClaimsFromContext(ctx)
	if claims == nil {
		return false
	}

	for _, p := range claims.Permissions {
		if p == permission {
			return true
		}
	}
	return false
}

// GetServiceID returns the authenticated service ID from context.
func GetServiceID(ctx context.Context) string {
	claims := ClaimsFromContext(ctx)
	if claims == nil {
		return ""
	}
	return claims.ServiceID
}

// GetServiceName returns the authenticated service name from context.
func GetServiceName(ctx context.Context) string {
	claims := ClaimsFromContext(ctx)
	if claims == nil {
		return ""
	}
	return claims.ServiceName
}

// claimsContextKeyType is used to store JWT claims in context.
type claimsContextKeyType struct{}

// TokenValidator provides an interface for custom token validation logic.
type TokenValidator interface {
	ValidateToken(ctx context.Context, claims *ServiceClaims) error
}

// RequirePermissions returns middleware that requires specific permissions.
func (b *Bundle) RequirePermissions(permissions ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFromContext(r.Context())
			if claims == nil {
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			// Check if service has required permissions
			for _, requiredPerm := range permissions {
				found := false
				for _, servicePerm := range claims.Permissions {
					if servicePerm == requiredPerm {
						found = true
						break
					}
				}
				if !found {
					http.Error(w, fmt.Sprintf("Permission required: %s", requiredPerm), http.StatusForbidden)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// GenerateServiceToken creates a token for this service to authenticate with other services.
func (b *Bundle) GenerateServiceToken(permissions []string) (string, error) {
	return b.GenerateToken(b.config.ServiceName, b.config.ServiceName, permissions)
}

// tokenCache provides thread-safe caching of service tokens.
type tokenCache struct {
	mu     sync.RWMutex
	tokens map[string]*cachedToken
}

// cachedToken represents a cached JWT token with expiry.
type cachedToken struct {
	token     string
	expiresAt time.Time
}

// newTokenCache creates a new token cache.
func newTokenCache() *tokenCache {
	cache := &tokenCache{
		tokens: make(map[string]*cachedToken),
	}

	// Start cleanup goroutine to remove expired tokens
	go cache.cleanup()

	return cache
}

// getOrGenerate gets a cached token or generates a new one if needed.
func (tc *tokenCache) getOrGenerate(key string, generator func() (string, time.Time, error)) (string, error) {
	tc.mu.RLock()
	if cached, exists := tc.tokens[key]; exists {
		// Check if token is still valid (with 5-minute buffer for clock skew)
		if time.Now().Add(5 * time.Minute).Before(cached.expiresAt) {
			tc.mu.RUnlock()
			return cached.token, nil
		}
	}
	tc.mu.RUnlock()

	// Generate new token
	token, expiresAt, err := generator()
	if err != nil {
		return "", err
	}

	// Cache the token
	tc.mu.Lock()
	tc.tokens[key] = &cachedToken{
		token:     token,
		expiresAt: expiresAt,
	}
	tc.mu.Unlock()

	return token, nil
}

// cleanup removes expired tokens from the cache.
func (tc *tokenCache) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		tc.mu.Lock()
		for key, cached := range tc.tokens {
			if now.After(cached.expiresAt) {
				delete(tc.tokens, key)
			}
		}
		tc.mu.Unlock()
	}
}

// validateServiceIdentifier validates service ID and name formats to prevent injection attacks.
func validateServiceIdentifier(identifier string) error {
	if identifier == "" {
		return fmt.Errorf("identifier cannot be empty")
	}

	// Service identifiers should only contain alphanumeric characters, hyphens, and underscores
	// This prevents injection attacks and ensures safe usage in logs and metrics
	validFormat := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validFormat.MatchString(identifier) {
		return fmt.Errorf("identifier contains invalid characters, only alphanumeric, hyphens, and underscores allowed")
	}

	// Reasonable length limits
	if len(identifier) > 64 {
		return fmt.Errorf("identifier too long, maximum 64 characters")
	}

	if len(identifier) < 2 {
		return fmt.Errorf("identifier too short, minimum 2 characters")
	}

	return nil
}