// Package main demonstrates a Forge microservice with Redis integration.
//
// This example shows how to:
//   - Use the Redis bundle for caching and messaging
//   - Implement Redis-backed business logic
//   - Use pub/sub for event-driven architecture
//   - Implement distributed locking
//   - Use Redis for rate limiting
//   - Provide Redis health checks
//
// # Prerequisites
//
// 1. Redis server running locally: redis-server
// 2. Optional: Redis CLI for testing: redis-cli
//
// # Run the service
//
//   REDIS_URL="redis://localhost:6379/0" go run main.go
//
// # Test the service
//
//   # Test caching
//   curl -X POST http://localhost:8081/api/cache/user/123 \
//     -H "Content-Type: application/json" \
//     -d '{"name": "John Doe", "email": "john@example.com"}'
//
//   curl http://localhost:8081/api/cache/user/123
//
//   # Test pub/sub
//   curl -X POST http://localhost:8081/api/events/user.created \
//     -H "Content-Type: application/json" \
//     -d '{"user_id": "123", "event": "user_created"}'
//
//   # Test rate limiting
//   for i in {1..10}; do curl http://localhost:8081/api/limited; done
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/datariot/forge/bundles/redis"
	"github.com/datariot/forge/config"
	"github.com/datariot/forge/framework"
	forgeHealth "github.com/datariot/forge/health"
)

// ServiceConfig extends BaseConfig with Redis-specific configuration.
type ServiceConfig struct {
	config.BaseConfig `yaml:",inline"`

	// Redis configuration
	RedisURL       string `yaml:"redis_url" env:"REDIS_URL"`
	CacheKeyPrefix string `yaml:"cache_key_prefix" env:"CACHE_KEY_PREFIX"`
	DefaultTTL     string `yaml:"default_ttl" env:"DEFAULT_TTL"`
}

// DefaultServiceConfig returns configuration with defaults.
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		BaseConfig:     config.DefaultBaseConfig(),
		CacheKeyPrefix: "forge-example",
		DefaultTTL:     "1h",
	}
}

// Validate validates the service configuration.
func (c *ServiceConfig) Validate() error {
	if err := c.BaseConfig.Validate(); err != nil {
		return err
	}

	if c.RedisURL == "" {
		return fmt.Errorf("REDIS_URL environment variable is required")
	}

	return nil
}

// CacheService demonstrates Redis caching functionality.
type CacheService struct {
	config      *ServiceConfig
	redisBundle *redis.Bundle
	rateLimiter *redis.RateLimiter
}

// NewCacheService creates a new cache service with Redis.
func NewCacheService(config *ServiceConfig, redisBundle *redis.Bundle) *CacheService {
	return &CacheService{
		config:      config,
		redisBundle: redisBundle,
		rateLimiter: redisBundle.NewRateLimiter("api"),
	}
}

// Start initializes the cache service.
func (s *CacheService) Start(ctx context.Context) error {
	log.Printf("CacheService started with Redis integration")

	// Start listening for pub/sub events
	go s.listenForEvents(ctx)

	return nil
}

// Stop gracefully shuts down the cache service.
func (s *CacheService) Stop(ctx context.Context) error {
	log.Printf("CacheService stopping...")
	return nil
}

// HealthChecks implements the HealthContributor interface.
func (s *CacheService) HealthChecks() []forgeHealth.Check {
	return []forgeHealth.Check{
		&CacheServiceHealthCheck{
			cache: s.redisBundle.Cache(),
		},
	}
}

// setupHTTPEndpoints configures HTTP endpoints for Redis operations.
func (s *CacheService) setupHTTPEndpoints(mux *http.ServeMux) {
	// Cache endpoints
	mux.HandleFunc("/api/cache/", s.handleCache)

	// Event publishing endpoint
	mux.HandleFunc("/api/events/", s.handleEvents)

	// Rate limited endpoint
	mux.HandleFunc("/api/limited", s.handleRateLimited)

	// Lock testing endpoint
	mux.HandleFunc("/api/lock/", s.handleDistributedLock)

	// Redis stats endpoint
	mux.HandleFunc("/api/redis/stats", s.handleRedisStats)
}

// User represents a user entity for caching demonstration.
type User struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// handleCache handles caching operations (GET/POST/DELETE).
func (s *CacheService) handleCache(w http.ResponseWriter, r *http.Request) {
	// Parse path: /api/cache/{type}/{id}
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/cache/"), "/")
	if len(pathParts) != 2 {
		http.Error(w, "Invalid cache path format", http.StatusBadRequest)
		return
	}

	cacheType := pathParts[0]
	id := pathParts[1]
	key := fmt.Sprintf("%s:%s:%s", s.config.CacheKeyPrefix, cacheType, id)

	switch r.Method {
	case http.MethodGet:
		s.handleCacheGet(w, r, key)
	case http.MethodPost:
		s.handleCacheSet(w, r, key, cacheType)
	case http.MethodDelete:
		s.handleCacheDelete(w, r, key)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleCacheGet retrieves an item from cache.
func (s *CacheService) handleCacheGet(w http.ResponseWriter, r *http.Request, key string) {
	var result interface{}
	err := s.redisBundle.Cache().Get(r.Context(), key, &result)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "Cache key not found", http.StatusNotFound)
		} else {
			http.Error(w, fmt.Sprintf("Cache error: %v", err), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"key":   key,
		"value": result,
		"found": true,
	})
}

// handleCacheSet stores an item in cache.
func (s *CacheService) handleCacheSet(w http.ResponseWriter, r *http.Request, key, cacheType string) {
	var data interface{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Parse TTL from query parameter or use default
	ttlStr := r.URL.Query().Get("ttl")
	if ttlStr == "" {
		ttlStr = s.config.DefaultTTL
	}

	ttl, err := time.ParseDuration(ttlStr)
	if err != nil {
		http.Error(w, "Invalid TTL format", http.StatusBadRequest)
		return
	}

	// Store in cache
	if err := s.redisBundle.Cache().Set(r.Context(), key, data, ttl); err != nil {
		http.Error(w, fmt.Sprintf("Cache error: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"key":    key,
		"stored": true,
		"ttl":    ttl.String(),
	})
}

// handleCacheDelete removes an item from cache.
func (s *CacheService) handleCacheDelete(w http.ResponseWriter, r *http.Request, key string) {
	if err := s.redisBundle.Cache().Delete(r.Context(), key); err != nil {
		http.Error(w, fmt.Sprintf("Cache error: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"key":     key,
		"deleted": true,
	})
}

// handleEvents handles pub/sub event publishing.
func (s *CacheService) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse path: /api/events/{channel}
	channel := strings.TrimPrefix(r.URL.Path, "/api/events/")
	if channel == "" {
		http.Error(w, "Channel name required", http.StatusBadRequest)
		return
	}

	var event interface{}
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Publish event
	if err := s.redisBundle.PubSub().Publish(r.Context(), channel, event); err != nil {
		http.Error(w, fmt.Sprintf("Publish error: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"channel":   channel,
		"published": true,
		"timestamp": time.Now().UTC(),
	})
}

// handleRateLimited demonstrates rate limiting.
func (s *CacheService) handleRateLimited(w http.ResponseWriter, r *http.Request) {
	clientIP := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		clientIP = strings.Split(forwarded, ",")[0]
	}

	// Allow 5 requests per minute per IP
	allowed, err := s.rateLimiter.Allow(r.Context(), clientIP, 5, 1*time.Minute)
	if err != nil {
		http.Error(w, fmt.Sprintf("Rate limit error: %v", err), http.StatusInternalServerError)
		return
	}

	if !allowed {
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":    "Request allowed",
		"client_ip":  clientIP,
		"timestamp":  time.Now().UTC(),
		"rate_limit": "5 requests per minute",
	})
}

// handleDistributedLock demonstrates distributed locking.
func (s *CacheService) handleDistributedLock(w http.ResponseWriter, r *http.Request) {
	// Parse path: /api/lock/{resource}
	resource := strings.TrimPrefix(r.URL.Path, "/api/lock/")
	if resource == "" {
		http.Error(w, "Resource name required", http.StatusBadRequest)
		return
	}

	lock := s.redisBundle.NewDistributedLock(resource, 30*time.Second)

	switch r.Method {
	case http.MethodPost:
		// Try to acquire lock
		acquired, err := lock.TryLock(r.Context())
		if err != nil {
			http.Error(w, fmt.Sprintf("Lock error: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"resource": resource,
			"acquired": acquired,
			"ttl":      "30s",
		})

	case http.MethodDelete:
		// Release lock
		if err := lock.Unlock(r.Context()); err != nil {
			http.Error(w, fmt.Sprintf("Unlock error: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"resource": resource,
			"released": true,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleRedisStats provides Redis connection and performance statistics.
func (s *CacheService) handleRedisStats(w http.ResponseWriter, r *http.Request) {
	client := s.redisBundle.Client()

	// Get Redis info
	info, err := client.Info(r.Context(), "server", "memory", "stats").Result()
	if err != nil {
		http.Error(w, fmt.Sprintf("Redis info error: %v", err), http.StatusInternalServerError)
		return
	}

	// Get connection pool stats
	stats := client.PoolStats()

	response := map[string]interface{}{
		"redis_info": strings.Split(info, "\r\n"),
		"pool_stats": map[string]interface{}{
			"hits":         stats.Hits,
			"misses":       stats.Misses,
			"timeouts":     stats.Timeouts,
			"total_conns":  stats.TotalConns,
			"idle_conns":   stats.IdleConns,
			"stale_conns":  stats.StaleConns,
		},
		"timestamp": time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// listenForEvents demonstrates pub/sub event listening with proper resource management.
func (s *CacheService) listenForEvents(ctx context.Context) {
	// Subscribe to user events with managed subscription
	subscription := s.redisBundle.PubSub().Subscribe(ctx, "user.*")
	defer subscription.Close() // Ensure proper cleanup

	log.Printf("Started listening for Redis pub/sub events on user.* channels")

	for {
		select {
		case msg := <-subscription.Messages():
			if msg == nil {
				log.Printf("Pub/sub subscription closed")
				return // Channel closed
			}

			log.Printf("Received event: channel=%s, payload=%s", msg.Channel, msg.Payload)

			// Process event (in a real application, you might parse JSON and handle different event types)
			var event map[string]interface{}
			if err := json.Unmarshal([]byte(msg.Payload), &event); err == nil {
				log.Printf("Processed event: %+v", event)
			}

		case <-ctx.Done():
			log.Printf("Stopping event listener")
			return
		}
	}
}

// CacheServiceHealthCheck provides service-specific health checking.
type CacheServiceHealthCheck struct {
	cache *redis.CacheService
}

// Name returns the health check name.
func (c *CacheServiceHealthCheck) Name() string {
	return "cache-service"
}

// Liveness performs a basic service health check.
func (c *CacheServiceHealthCheck) Liveness(ctx context.Context) error {
	// Test basic cache operation
	testKey := "health:liveness:test"
	return c.cache.Set(ctx, testKey, "ok", 10*time.Second)
}

// Readiness performs a comprehensive service readiness check.
func (c *CacheServiceHealthCheck) Readiness(ctx context.Context) error {
	// Test cache operations
	testKey := "health:readiness:test"
	testValue := map[string]interface{}{
		"timestamp": time.Now().UTC(),
		"service":   "cache-service",
	}

	// Test set operation
	if err := c.cache.Set(ctx, testKey, testValue, 30*time.Second); err != nil {
		return fmt.Errorf("cache set operation failed: %w", err)
	}

	// Test get operation
	var retrieved map[string]interface{}
	if err := c.cache.Get(ctx, testKey, &retrieved); err != nil {
		return fmt.Errorf("cache get operation failed: %w", err)
	}

	// Test delete operation
	if err := c.cache.Delete(ctx, testKey); err != nil {
		return fmt.Errorf("cache delete operation failed: %w", err)
	}

	return nil
}

func main() {
	// Load configuration
	cfg := DefaultServiceConfig()
	cfg.ServiceName = "redis-service"

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Configuration validation failed: %v", err)
	}

	// Create Redis bundle
	redisConfig := redis.Config{
		RedisURL:           cfg.RedisURL,
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

	redisBundle := redis.NewBundle(redisConfig)

	// Create cache service
	cacheService := NewCacheService(&cfg, redisBundle)

	// Create the application with Redis integration
	app, err := framework.New(
		framework.WithConfig(&cfg.BaseConfig),
		framework.WithVersion("1.0.0"),
		framework.WithBundle(redisBundle),
		framework.WithComponent(cacheService),
		framework.WithHealthContributor(cacheService),
		framework.WithStartupHook(func(ctx context.Context, app *framework.App) error {
			// Setup HTTP endpoints for Redis operations
			// Note: In a real application, you might integrate this with the HTTP server builder
			log.Printf("Redis service endpoints available:")
			log.Printf("  GET    /api/cache/{type}/{id} - Get cached item")
			log.Printf("  POST   /api/cache/{type}/{id} - Set cached item")
			log.Printf("  DELETE /api/cache/{type}/{id} - Delete cached item")
			log.Printf("  POST   /api/events/{channel} - Publish event")
			log.Printf("  GET    /api/limited - Rate limited endpoint")
			log.Printf("  POST   /api/lock/{resource} - Acquire distributed lock")
			log.Printf("  DELETE /api/lock/{resource} - Release distributed lock")
			log.Printf("  GET    /api/redis/stats - Redis statistics")
			return nil
		}),
	)
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}

	log.Printf("Starting %s with Redis integration...", cfg.ServiceName)
	log.Printf("Redis URL: %s", cfg.RedisURL)
	log.Printf("Cache key prefix: %s", cfg.CacheKeyPrefix)

	// Run the application
	if err := app.Run(context.Background()); err != nil {
		log.Fatalf("Application failed: %v", err)
	}
}