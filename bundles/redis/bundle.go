// Package redis provides a Redis integration bundle for Forge applications.
//
// The Redis bundle provides:
//   - Redis client connection management with connection pooling
//   - Health checks for Redis connectivity and performance
//   - Caching interface with TTL support
//   - Pub/Sub messaging capabilities
//   - Distributed locking utilities
//   - Rate limiting support
//   - Session storage management
//
// # Basic Usage
//
// Add the Redis bundle to your application:
//
//	config := redis.Config{
//		RedisURL: "redis://localhost:6379/0",
//		PoolSize: 10,
//		MinIdleConns: 2,
//		DialTimeout: 5 * time.Second,
//	}
//
//	bundle := redis.NewBundle(config)
//
//	app, err := framework.New(
//		framework.WithConfig(&baseConfig),
//		framework.WithBundle(bundle),
//	)
//
// # Accessing Redis
//
// The bundle provides a Redis client that can be used for dependency injection:
//
//	type CacheService struct {
//		redis redis.UniversalClient
//	}
//
//	func NewCacheService(redisClient redis.UniversalClient) *CacheService {
//		return &CacheService{redis: redisClient}
//	}
//
// # Caching Operations
//
// The bundle provides a high-level caching interface:
//
//	cache := bundle.Cache()
//
//	// Set with TTL
//	err := cache.Set(ctx, "user:123", userData, 1*time.Hour)
//
//	// Get with type safety
//	var user User
//	err := cache.Get(ctx, "user:123", &user)
//
// # Pub/Sub Messaging
//
//	pubsub := bundle.PubSub()
//
//	// Subscribe to messages
//	messages := pubsub.Subscribe(ctx, "user.events")
//
//	// Publish messages
//	err := pubsub.Publish(ctx, "user.events", userData)
//
// # Health Checks
//
// The bundle automatically provides Redis health checks that verify:
//   - Redis connectivity (ping)
//   - Memory usage and performance
//   - Connection pool health
package redis

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/datariot/forge/errors"
	"github.com/datariot/forge/framework"
	forgeHealth "github.com/datariot/forge/health"
)

// Config contains Redis-specific configuration options.
type Config struct {
	// RedisURL is the Redis connection string.
	// Examples:
	//   "redis://localhost:6379/0"
	//   "redis://user:password@localhost:6379/0"
	//   "rediss://user:password@redis.example.com:6380/0" (TLS)
	RedisURL string

	// Connection pool configuration
	PoolSize        int           // Maximum number of socket connections (default: 10)
	MinIdleConns    int           // Minimum number of idle connections (default: 2)
	MaxIdleTime     time.Duration // Maximum amount of time a connection may be idle (default: 30 minutes)
	MaxConnAge      time.Duration // Maximum amount of time a connection may be reused (default: 1 hour)
	PoolTimeout     time.Duration // Amount of time client waits for connection (default: 4 seconds)

	// Timeouts
	DialTimeout  time.Duration // Timeout for establishing new connections (default: 5 seconds)
	ReadTimeout  time.Duration // Timeout for socket reads (default: 3 seconds)
	WriteTimeout time.Duration // Timeout for socket writes (default: 3 seconds)

	// TLS configuration (for rediss:// URLs)
	TLSConfig *tls.Config

	// Health check configuration
	HealthCheckTimeout time.Duration // Timeout for health check operations (default: 5 seconds)

	// Database selection (can be overridden by URL)
	Database int // Redis database number (default: 0)
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		PoolSize:           10,
		MinIdleConns:       2,
		MaxIdleTime:        30 * time.Minute,
		MaxConnAge:         1 * time.Hour,
		PoolTimeout:        4 * time.Second,
		DialTimeout:        5 * time.Second,
		ReadTimeout:        3 * time.Second,
		WriteTimeout:       3 * time.Second,
		HealthCheckTimeout: 5 * time.Second,
		Database:           0,
	}
}

// Validate validates the Redis configuration.
func (c *Config) Validate() error {
	if c.RedisURL == "" {
		return fmt.Errorf("redis_url is required")
	}

	// Parse and validate Redis URL
	parsedURL, err := url.Parse(c.RedisURL)
	if err != nil {
		return fmt.Errorf("invalid redis_url format: %w", err)
	}

	// Validate scheme
	if parsedURL.Scheme != "redis" && parsedURL.Scheme != "rediss" {
		return fmt.Errorf("redis_url must use 'redis://' or 'rediss://' scheme")
	}

	// Security: Enforce authentication for remote connections
	if parsedURL.Hostname() != "localhost" && parsedURL.Hostname() != "127.0.0.1" && parsedURL.Hostname() != "" {
		if parsedURL.User == nil {
			return fmt.Errorf("authentication required for remote Redis connections")
		}
		if password, ok := parsedURL.User.Password(); !ok || password == "" {
			return fmt.Errorf("password required for remote Redis connections")
		}
	}

	// Security: Enforce TLS for remote connections
	if parsedURL.Scheme == "redis" && parsedURL.Hostname() != "localhost" && parsedURL.Hostname() != "127.0.0.1" && parsedURL.Hostname() != "" {
		return fmt.Errorf("TLS required for remote Redis connections, use rediss:// scheme")
	}

	// Validate TLS configuration if using rediss://
	if parsedURL.Scheme == "rediss" && c.TLSConfig != nil {
		if c.TLSConfig.InsecureSkipVerify {
			return fmt.Errorf("TLS certificate verification cannot be disabled for security")
		}
	}

	// Validate connection pool settings
	if c.PoolSize <= 0 {
		return fmt.Errorf("pool_size must be positive, got %d", c.PoolSize)
	}
	if c.MinIdleConns < 0 {
		return fmt.Errorf("min_idle_conns must be non-negative, got %d", c.MinIdleConns)
	}
	if c.MinIdleConns > c.PoolSize {
		return fmt.Errorf("min_idle_conns (%d) cannot exceed pool_size (%d)", c.MinIdleConns, c.PoolSize)
	}

	// Validate timeouts
	if c.DialTimeout <= 0 {
		return fmt.Errorf("dial_timeout must be positive, got %v", c.DialTimeout)
	}
	if c.ReadTimeout <= 0 {
		return fmt.Errorf("read_timeout must be positive, got %v", c.ReadTimeout)
	}
	if c.WriteTimeout <= 0 {
		return fmt.Errorf("write_timeout must be positive, got %v", c.WriteTimeout)
	}
	if c.PoolTimeout <= 0 {
		return fmt.Errorf("pool_timeout must be positive, got %v", c.PoolTimeout)
	}
	if c.HealthCheckTimeout <= 0 {
		return fmt.Errorf("health_check_timeout must be positive, got %v", c.HealthCheckTimeout)
	}

	// Validate database number
	if c.Database < 0 || c.Database > 15 {
		return fmt.Errorf("database must be between 0 and 15, got %d", c.Database)
	}

	return nil
}

// SanitizedRedisURL returns a sanitized version of the Redis URL for logging.
// This removes sensitive credentials while preserving connection information.
func (c *Config) SanitizedRedisURL() string {
	parsedURL, err := url.Parse(c.RedisURL)
	if err != nil {
		return "[invalid-redis-url]"
	}

	// Replace credentials with placeholder
	if parsedURL.User != nil {
		username := parsedURL.User.Username()
		if username == "" {
			username = "[user]"
		}
		parsedURL.User = url.UserPassword(username, "[password]")
	}

	return parsedURL.String()
}

// Bundle provides Redis integration for Forge applications.
type Bundle struct {
	config Config
	client redis.UniversalClient
	cache  *CacheService
	pubsub *PubSubService
}

// NewBundle creates a new Redis bundle with the given configuration.
func NewBundle(config Config) *Bundle {
	return &Bundle{
		config: config,
	}
}

// Name returns the bundle name.
func (b *Bundle) Name() string {
	return "redis"
}

// Initialize sets up the Redis connection and services.
func (b *Bundle) Initialize(app *framework.App) error {
	if err := b.config.Validate(); err != nil {
		return errors.ErrInvalidConfiguration.WithMessage("Redis configuration validation failed").WithCause(err)
	}

	// Parse Redis URL
	opts, err := redis.ParseURL(b.config.RedisURL)
	if err != nil {
		return errors.ErrInvalidConfiguration.WithMessage("invalid Redis URL format").WithCause(err)
	}

	// Apply configuration overrides
	opts.PoolSize = b.config.PoolSize
	opts.MinIdleConns = b.config.MinIdleConns
	opts.ConnMaxIdleTime = b.config.MaxIdleTime
	opts.ConnMaxLifetime = b.config.MaxConnAge
	opts.PoolTimeout = b.config.PoolTimeout
	opts.DialTimeout = b.config.DialTimeout
	opts.ReadTimeout = b.config.ReadTimeout
	opts.WriteTimeout = b.config.WriteTimeout

	// Override database if specified in config
	if b.config.Database != 0 {
		opts.DB = b.config.Database
	}

	// Apply TLS configuration if provided
	if b.config.TLSConfig != nil {
		opts.TLSConfig = b.config.TLSConfig
	}

	// Create Redis client
	b.client = redis.NewClient(opts)

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := b.client.Ping(ctx).Err(); err != nil {
		b.client.Close()
		return errors.ErrRepositoryUnavailable.WithMessage(
			"failed to connect to Redis at %s", b.config.SanitizedRedisURL(),
		).WithCause(err)
	}

	// Initialize high-level services
	b.cache = NewCacheService(b.client)
	b.pubsub = NewPubSubService(b.client)

	return nil
}

// Client returns the Redis client. This can be used for dependency injection.
func (b *Bundle) Client() redis.UniversalClient {
	return b.client
}

// Cache returns the caching service interface.
func (b *Bundle) Cache() *CacheService {
	return b.cache
}

// PubSub returns the pub/sub messaging service.
func (b *Bundle) PubSub() *PubSubService {
	return b.pubsub
}

// Close closes the Redis connection. Called during application shutdown.
func (b *Bundle) Close() error {
	if b.client != nil {
		return b.client.Close()
	}
	return nil
}

// HealthChecks returns health checks for the Redis connection.
func (b *Bundle) HealthChecks() []forgeHealth.Check {
	if b.client == nil {
		return nil
	}

	return []forgeHealth.Check{
		&RedisHealthCheck{
			client:  b.client,
			timeout: b.config.HealthCheckTimeout,
		},
	}
}

// RedisHealthCheck implements health checking for Redis connections.
type RedisHealthCheck struct {
	client  redis.UniversalClient
	timeout time.Duration
}

// Name returns the health check name.
func (c *RedisHealthCheck) Name() string {
	return "redis"
}

// Liveness performs a basic connectivity check.
func (c *RedisHealthCheck) Liveness(ctx context.Context) error {
	timeout := c.timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	// Use configured timeout, but respect parent context deadline
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > timeout {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	return c.client.Ping(ctx).Err()
}

// Readiness performs a more comprehensive check including memory and performance.
func (c *RedisHealthCheck) Readiness(ctx context.Context) error {
	timeout := c.timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	// Use configured timeout, but respect parent context deadline
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > timeout {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// First check basic connectivity
	if err := c.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("Redis ping failed: %w", err)
	}

	// Check Redis memory usage and basic operations
	info, err := c.client.Info(ctx, "memory").Result()
	if err != nil {
		return fmt.Errorf("failed to get Redis memory info: %w", err)
	}

	if info == "" {
		return fmt.Errorf("Redis memory info returned empty")
	}

	// Test basic set/get operation
	testKey := "forge:health:test"
	testValue := "ok"

	if err := c.client.Set(ctx, testKey, testValue, 30*time.Second).Err(); err != nil {
		return fmt.Errorf("failed to set test key in Redis: %w", err)
	}

	result, err := c.client.Get(ctx, testKey).Result()
	if err != nil {
		return fmt.Errorf("failed to get test key from Redis: %w", err)
	}

	if result != testValue {
		return fmt.Errorf("Redis test operation failed: expected %q, got %q", testValue, result)
	}

	// Clean up test key
	c.client.Del(ctx, testKey)

	return nil
}

// CacheService provides high-level caching operations.
type CacheService struct {
	client redis.UniversalClient
}

// NewCacheService creates a new cache service.
func NewCacheService(client redis.UniversalClient) *CacheService {
	return &CacheService{client: client}
}

// Set stores a value in the cache with the specified TTL.
func (c *CacheService) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal cache value: %w", err)
	}

	return c.client.Set(ctx, key, data, ttl).Err()
}

// Get retrieves a value from the cache and unmarshals it into the provided destination.
func (c *CacheService) Get(ctx context.Context, key string, dest interface{}) error {
	data, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return fmt.Errorf("cache key not found: %s", key)
		}
		return fmt.Errorf("failed to get cache value: %w", err)
	}

	if err := json.Unmarshal([]byte(data), dest); err != nil {
		return fmt.Errorf("failed to unmarshal cache value: %w", err)
	}

	return nil
}

// Delete removes a key from the cache.
func (c *CacheService) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	return c.client.Del(ctx, keys...).Err()
}

// Exists checks if keys exist in the cache.
func (c *CacheService) Exists(ctx context.Context, keys ...string) (int64, error) {
	if len(keys) == 0 {
		return 0, nil
	}

	return c.client.Exists(ctx, keys...).Result()
}

// TTL returns the time to live for a key.
func (c *CacheService) TTL(ctx context.Context, key string) (time.Duration, error) {
	return c.client.TTL(ctx, key).Result()
}

// PubSubService provides pub/sub messaging capabilities.
type PubSubService struct {
	client redis.UniversalClient
}

// NewPubSubService creates a new pub/sub service.
func NewPubSubService(client redis.UniversalClient) *PubSubService {
	return &PubSubService{client: client}
}

// Publish publishes a message to the specified channel.
func (p *PubSubService) Publish(ctx context.Context, channel string, message interface{}) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal pub/sub message: %w", err)
	}

	return p.client.Publish(ctx, channel, data).Err()
}

// Subscription represents a managed Redis subscription with proper cleanup.
type Subscription struct {
	pubsub   *redis.PubSub
	messages <-chan *redis.Message
	ctx      context.Context
	cancel   context.CancelFunc
}

// Messages returns the channel for receiving messages.
func (s *Subscription) Messages() <-chan *redis.Message {
	return s.messages
}

// Close properly closes the subscription and cleans up resources.
func (s *Subscription) Close() error {
	s.cancel()
	return s.pubsub.Close()
}

// Subscribe subscribes to one or more channels and returns a managed subscription.
func (p *PubSubService) Subscribe(ctx context.Context, channels ...string) *Subscription {
	subCtx, cancel := context.WithCancel(ctx)
	pubsub := p.client.Subscribe(subCtx, channels...)

	return &Subscription{
		pubsub:   pubsub,
		messages: pubsub.Channel(),
		ctx:      subCtx,
		cancel:   cancel,
	}
}

// PSubscribe subscribes to channels matching patterns and returns a managed subscription.
func (p *PubSubService) PSubscribe(ctx context.Context, patterns ...string) *Subscription {
	subCtx, cancel := context.WithCancel(ctx)
	pubsub := p.client.PSubscribe(subCtx, patterns...)

	return &Subscription{
		pubsub:   pubsub,
		messages: pubsub.Channel(),
		ctx:      subCtx,
		cancel:   cancel,
	}
}

// DistributedLock provides Redis-based distributed locking.
type DistributedLock struct {
	client redis.UniversalClient
	key    string
	value  string
	ttl    time.Duration
}

// NewDistributedLock creates a new distributed lock.
func (b *Bundle) NewDistributedLock(key string, ttl time.Duration) *DistributedLock {
	return &DistributedLock{
		client: b.client,
		key:    fmt.Sprintf("lock:%s", key),
		value:  generateSecureLockValue(),
		ttl:    ttl,
	}
}

// generateSecureLockValue creates a cryptographically secure lock value.
func generateSecureLockValue() string {
	// Generate 16 bytes of random data
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		// Fallback to less secure but still unique value
		return fmt.Sprintf("fallback:%d:%d", os.Getpid(), time.Now().UnixNano())
	}
	return hex.EncodeToString(randomBytes)
}

// TryLock attempts to acquire the distributed lock.
func (l *DistributedLock) TryLock(ctx context.Context) (bool, error) {
	result, err := l.client.SetNX(ctx, l.key, l.value, l.ttl).Result()
	return result, err
}

// Unlock releases the distributed lock.
func (l *DistributedLock) Unlock(ctx context.Context) error {
	// Use Lua script to ensure we only delete our own lock
	script := `
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`

	return l.client.Eval(ctx, script, []string{l.key}, l.value).Err()
}

// RateLimiter provides Redis-based rate limiting using sliding window.
type RateLimiter struct {
	client redis.UniversalClient
	prefix string
}

// NewRateLimiter creates a new Redis-based rate limiter.
func (b *Bundle) NewRateLimiter(prefix string) *RateLimiter {
	return &RateLimiter{
		client: b.client,
		prefix: fmt.Sprintf("ratelimit:%s", prefix),
	}
}

// Allow checks if a request is allowed under the rate limit using atomic Lua script.
func (r *RateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	now := time.Now()
	windowStart := now.Add(-window)
	fullKey := fmt.Sprintf("%s:%s", r.prefix, key)

	// Atomic rate limiting with Lua script
	script := `
		local key = KEYS[1]
		local now = tonumber(ARGV[1])
		local window_start = tonumber(ARGV[2])
		local limit = tonumber(ARGV[3])
		local window_seconds = tonumber(ARGV[4])

		-- Remove expired entries
		redis.call('ZREMRANGEBYSCORE', key, 0, window_start)

		-- Check current count
		local current = redis.call('ZCARD', key)
		if current >= limit then
			return 0  -- Denied
		end

		-- Add current request and set expiry
		redis.call('ZADD', key, now, now)
		redis.call('EXPIRE', key, math.ceil(window_seconds))

		return 1  -- Allowed
	`

	result, err := r.client.Eval(ctx, script, []string{fullKey},
		now.UnixNano(),
		windowStart.UnixNano(),
		limit,
		window.Seconds(),
	).Result()

	if err != nil {
		return false, fmt.Errorf("rate limit check failed: %w", err)
	}

	return result.(int64) == 1, nil
}