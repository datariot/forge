package prometheus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// newTestRegistry creates an isolated Prometheus registry for each test to prevent
// "already registered" panics when tests run in parallel or sequence.
func newTestRegistry() *prometheus.Registry {
	return prometheus.NewRegistry()
}

// newInitializedBundle creates a Bundle with a fresh isolated registry,
// validates and initializes it, and returns the bundle.
func newInitializedBundle(t *testing.T, cfg Config) *Bundle {
	t.Helper()
	reg := newTestRegistry()
	cfg.Registry = reg
	b := NewBundle(cfg)
	// Initialize requires a framework.App which has complex dependencies.
	// Directly initialize metrics and set up registry to test the recording logic.
	b.registry = reg
	b.gatherer = reg
	if err := b.initializeMetrics(); err != nil {
		t.Fatalf("initializeMetrics failed: %v", err)
	}
	return b
}

// --- Config.Validate tests ---

func TestConfigValidate_Valid(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Namespace = "myservice"
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid config, got error: %v", err)
	}
}

func TestConfigValidate_MissingNamespace(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Namespace = ""
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing namespace")
	}
}

func TestConfigValidate_InvalidNamespace(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Namespace = "123invalid" // starts with digit
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid namespace")
	}
}

func TestConfigValidate_ValidSubsystem(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Namespace = "svc"
	cfg.Subsystem = "api"
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid config with subsystem, got: %v", err)
	}
}

func TestConfigValidate_InvalidSubsystem(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Namespace = "svc"
	cfg.Subsystem = "123bad"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid subsystem")
	}
}

func TestConfigValidate_TooManyServiceLabels(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Namespace = "svc"
	for i := 0; i < 11; i++ {
		cfg.ServiceLabels[string(rune('a'+i))+"key"] = "val"
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for too many service labels")
	}
}

func TestConfigValidate_InvalidServiceLabelName(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Namespace = "svc"
	cfg.ServiceLabels["123invalid"] = "val"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid label name")
	}
}

func TestConfigValidate_SensitiveLabelName(t *testing.T) {
	tests := []string{"password_field", "api_secret", "auth_token"}
	for _, name := range tests {
		cfg := DefaultConfig()
		cfg.Namespace = "svc"
		cfg.ServiceLabels[name] = "val"
		if err := cfg.Validate(); err == nil {
			t.Errorf("expected error for sensitive label name %q", name)
		}
	}
}

func TestConfigValidate_ServiceLabelValueTooLong(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Namespace = "svc"
	longVal := make([]byte, 257)
	for i := range longVal {
		longVal[i] = 'x'
	}
	cfg.ServiceLabels["env"] = string(longVal)
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for too-long label value")
	}
}

func TestConfigValidate_EmptyHistogramBuckets(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Namespace = "svc"
	cfg.HistogramBuckets = []float64{}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for empty histogram buckets")
	}
}

func TestConfigValidate_TooManyHistogramBuckets(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Namespace = "svc"
	buckets := make([]float64, 51)
	for i := range buckets {
		buckets[i] = float64(i + 1)
	}
	cfg.HistogramBuckets = buckets
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for too many histogram buckets")
	}
}

func TestConfigValidate_BucketsNotAscending(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Namespace = "svc"
	cfg.HistogramBuckets = []float64{0.5, 0.1, 1.0}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for non-ascending buckets")
	}
}

func TestConfigValidate_BucketNotPositive(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Namespace = "svc"
	cfg.HistogramBuckets = []float64{0.0, 1.0}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for non-positive first bucket")
	}
}

// --- DefaultConfig tests ---

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.EnableDefaultMetrics != true {
		t.Error("expected EnableDefaultMetrics=true")
	}
	if cfg.EnableHTTPMetrics != true {
		t.Error("expected EnableHTTPMetrics=true")
	}
	if cfg.EnableGRPCMetrics != true {
		t.Error("expected EnableGRPCMetrics=true")
	}
	if len(cfg.HistogramBuckets) == 0 {
		t.Error("expected non-empty HistogramBuckets")
	}
	if cfg.ServiceLabels == nil {
		t.Error("expected ServiceLabels to be initialized")
	}
}

// --- RecordHTTPRequest tests ---

func TestRecordHTTPRequest(t *testing.T) {
	cfg := Config{
		Namespace:         "test",
		EnableHTTPMetrics: true,
		HistogramBuckets:  []float64{0.01, 0.1, 1.0},
		ServiceLabels:     map[string]string{},
	}
	b := newInitializedBundle(t, cfg)

	// Should not panic
	b.RecordHTTPRequest("GET", "/api/v1/users", 200, 50*time.Millisecond)
	b.RecordHTTPRequest("POST", "/api/v1/users", 201, 100*time.Millisecond)
	b.RecordHTTPRequest("GET", "/api/v1/users", 500, 200*time.Millisecond)
}

func TestRecordHTTPRequest_MetricsDisabled(t *testing.T) {
	cfg := Config{
		Namespace:         "test",
		EnableHTTPMetrics: false,
		HistogramBuckets:  []float64{0.01, 0.1, 1.0},
		ServiceLabels:     map[string]string{},
	}
	b := newInitializedBundle(t, cfg)

	// httpRequestsTotal and httpRequestDuration are nil — should not panic
	b.RecordHTTPRequest("GET", "/api", 200, time.Millisecond)
}

// --- RecordGRPCRequest tests ---

func TestRecordGRPCRequest(t *testing.T) {
	cfg := Config{
		Namespace:         "test",
		EnableGRPCMetrics: true,
		HistogramBuckets:  []float64{0.01, 0.1, 1.0},
		ServiceLabels:     map[string]string{},
	}
	b := newInitializedBundle(t, cfg)

	b.RecordGRPCRequest("/myservice.MyService/GetUser", "OK", 10*time.Millisecond)
	b.RecordGRPCRequest("/myservice.MyService/CreateUser", "InvalidArgument", 5*time.Millisecond)
}

func TestRecordGRPCRequest_MetricsDisabled(t *testing.T) {
	cfg := Config{
		Namespace:         "test",
		EnableGRPCMetrics: false,
		HistogramBuckets:  []float64{0.01, 0.1, 1.0},
		ServiceLabels:     map[string]string{},
	}
	b := newInitializedBundle(t, cfg)
	b.RecordGRPCRequest("/svc/Method", "OK", time.Millisecond)
}

// --- RecordHealthCheck tests ---

func TestRecordHealthCheck(t *testing.T) {
	cfg := Config{
		Namespace:           "test",
		EnableHealthMetrics: true,
		HistogramBuckets:    []float64{0.001, 0.01, 0.1},
		ServiceLabels:       map[string]string{},
	}
	b := newInitializedBundle(t, cfg)

	b.RecordHealthCheck("database", "readiness", true, 5*time.Millisecond)
	b.RecordHealthCheck("redis", "liveness", false, 2*time.Millisecond)
}

func TestRecordHealthCheck_MetricsDisabled(t *testing.T) {
	cfg := Config{
		Namespace:           "test",
		EnableHealthMetrics: false,
		HistogramBuckets:    []float64{0.001, 0.01, 0.1},
		ServiceLabels:       map[string]string{},
	}
	b := newInitializedBundle(t, cfg)
	b.RecordHealthCheck("db", "readiness", true, time.Millisecond)
}

// --- Connection metrics tests ---

func TestUpdateDatabaseConnections(t *testing.T) {
	cfg := Config{
		Namespace:           "test",
		EnableBundleMetrics: true,
		HistogramBuckets:    []float64{0.01, 0.1},
		ServiceLabels:       map[string]string{},
	}
	b := newInitializedBundle(t, cfg)

	b.UpdateDatabaseConnections(5, 3)
	b.UpdateDatabaseConnections(0, 10)
}

func TestUpdateRedisConnections(t *testing.T) {
	cfg := Config{
		Namespace:           "test",
		EnableBundleMetrics: true,
		HistogramBuckets:    []float64{0.01, 0.1},
		ServiceLabels:       map[string]string{},
	}
	b := newInitializedBundle(t, cfg)

	b.UpdateRedisConnections(2, 8)
}

func TestUpdateDatabaseConnections_MetricsDisabled(t *testing.T) {
	cfg := Config{
		Namespace:           "test",
		EnableBundleMetrics: false,
		HistogramBuckets:    []float64{0.01, 0.1},
		ServiceLabels:       map[string]string{},
	}
	b := newInitializedBundle(t, cfg)
	// nil gauges — should not panic
	b.UpdateDatabaseConnections(5, 3)
	b.UpdateRedisConnections(1, 2)
}

// --- JWT and circuit breaker metrics ---

func TestRecordJWTValidation(t *testing.T) {
	cfg := Config{
		Namespace:           "test",
		EnableBundleMetrics: true,
		HistogramBuckets:    []float64{0.01, 0.1},
		ServiceLabels:       map[string]string{},
	}
	b := newInitializedBundle(t, cfg)

	b.RecordJWTValidation(true, "auth-service")
	b.RecordJWTValidation(false, "auth-service")
}

func TestRecordJWTValidation_MetricsDisabled(t *testing.T) {
	cfg := Config{
		Namespace:           "test",
		EnableBundleMetrics: false,
		HistogramBuckets:    []float64{0.01, 0.1},
		ServiceLabels:       map[string]string{},
	}
	b := newInitializedBundle(t, cfg)
	b.RecordJWTValidation(true, "svc")
}

func TestUpdateCircuitBreakerState(t *testing.T) {
	cfg := Config{
		Namespace:           "test",
		EnableBundleMetrics: true,
		HistogramBuckets:    []float64{0.01, 0.1},
		ServiceLabels:       map[string]string{},
	}
	b := newInitializedBundle(t, cfg)

	b.UpdateCircuitBreakerState("payment-service", 0) // closed
	b.UpdateCircuitBreakerState("payment-service", 1) // half-open
	b.UpdateCircuitBreakerState("payment-service", 2) // open
}

func TestUpdateCircuitBreakerState_MetricsDisabled(t *testing.T) {
	cfg := Config{
		Namespace:           "test",
		EnableBundleMetrics: false,
		HistogramBuckets:    []float64{0.01, 0.1},
		ServiceLabels:       map[string]string{},
	}
	b := newInitializedBundle(t, cfg)
	b.UpdateCircuitBreakerState("svc", 0)
}

// --- Custom metric creation ---

func TestCreateCustomCounter(t *testing.T) {
	cfg := Config{
		Namespace:        "test",
		HistogramBuckets: []float64{0.01, 0.1},
		ServiceLabels:    map[string]string{},
	}
	b := newInitializedBundle(t, cfg)

	counter, err := b.CreateCustomCounter("requests_total", "Total requests", []string{"method"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if counter == nil {
		t.Error("expected non-nil counter")
	}
	counter.WithLabelValues("GET").Inc()
}

func TestCreateCustomCounter_InvalidName(t *testing.T) {
	cfg := Config{
		Namespace:        "test",
		HistogramBuckets: []float64{0.01, 0.1},
		ServiceLabels:    map[string]string{},
	}
	b := newInitializedBundle(t, cfg)

	_, err := b.CreateCustomCounter("123bad", "help", []string{})
	if err == nil {
		t.Error("expected error for invalid metric name")
	}
}

func TestCreateCustomCounter_EmptyHelp(t *testing.T) {
	cfg := Config{
		Namespace:        "test",
		HistogramBuckets: []float64{0.01, 0.1},
		ServiceLabels:    map[string]string{},
	}
	b := newInitializedBundle(t, cfg)

	_, err := b.CreateCustomCounter("valid_name", "", []string{})
	if err == nil {
		t.Error("expected error for empty help")
	}
}

func TestCreateCustomGauge(t *testing.T) {
	cfg := Config{
		Namespace:        "test",
		HistogramBuckets: []float64{0.01, 0.1},
		ServiceLabels:    map[string]string{},
	}
	b := newInitializedBundle(t, cfg)

	gauge, err := b.CreateCustomGauge("queue_depth", "Queue depth", []string{"queue"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if gauge == nil {
		t.Error("expected non-nil gauge")
	}
	gauge.WithLabelValues("high").Set(42)
}

func TestCreateCustomHistogram(t *testing.T) {
	cfg := Config{
		Namespace:        "test",
		HistogramBuckets: []float64{0.01, 0.1, 1.0},
		ServiceLabels:    map[string]string{},
	}
	b := newInitializedBundle(t, cfg)

	hist, err := b.CreateCustomHistogram("response_size_bytes", "Response size", []string{"endpoint"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if hist == nil {
		t.Error("expected non-nil histogram")
	}
	hist.WithLabelValues("/api").Observe(1024)
}

func TestCreateCustomSummary(t *testing.T) {
	cfg := Config{
		Namespace:        "test",
		HistogramBuckets: []float64{0.01, 0.1, 1.0},
		ServiceLabels:    map[string]string{},
	}
	b := newInitializedBundle(t, cfg)

	objectives := map[float64]float64{0.5: 0.05, 0.9: 0.01}
	summary, err := b.CreateCustomSummary("latency_summary", "Latency summary", []string{"method"}, objectives)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if summary == nil {
		t.Error("expected non-nil summary")
	}
	summary.WithLabelValues("GET").Observe(0.05)
}

func TestCreateCustomCounter_TooManyLabels(t *testing.T) {
	cfg := Config{
		Namespace:        "test",
		HistogramBuckets: []float64{0.01, 0.1},
		ServiceLabels:    map[string]string{},
	}
	b := newInitializedBundle(t, cfg)

	labels := make([]string, 16)
	for i := range labels {
		labels[i] = "label" + string(rune('a'+i))
	}
	_, err := b.CreateCustomCounter("valid_name", "help text", labels)
	if err == nil {
		t.Error("expected error for too many labels")
	}
}

// --- Metrics handler tests ---

func TestGetMetricsHandler(t *testing.T) {
	cfg := Config{
		Namespace:         "test",
		EnableHTTPMetrics: true,
		HistogramBuckets:  []float64{0.01, 0.1},
		ServiceLabels:     map[string]string{},
	}
	b := newInitializedBundle(t, cfg)

	handler := b.GetMetricsHandler()
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestGetSecureMetricsHandler_BasicAuth(t *testing.T) {
	cfg := Config{
		Namespace:        "test",
		HistogramBuckets: []float64{0.01, 0.1},
		ServiceLabels:    map[string]string{},
	}
	b := newInitializedBundle(t, cfg)

	secConfig := SecurityConfig{
		MaxRequestsInFlight: 3,
		Timeout:             10 * time.Second,
		EnableBasicAuth:     true,
		Username:            "admin",
		Password:            "secret",
	}
	handler := b.GetSecureMetricsHandler(secConfig)

	t.Run("missing credentials returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", w.Code)
		}
	})

	t.Run("wrong credentials returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		req.SetBasicAuth("admin", "wrong")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", w.Code)
		}
	})

	t.Run("correct credentials returns 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		req.SetBasicAuth("admin", "secret")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})
}

func TestDefaultSecurityConfig(t *testing.T) {
	cfg := DefaultSecurityConfig()
	if cfg.MaxRequestsInFlight != 3 {
		t.Errorf("expected 3, got %d", cfg.MaxRequestsInFlight)
	}
	if cfg.Timeout != 10*time.Second {
		t.Errorf("expected 10s, got %v", cfg.Timeout)
	}
	if cfg.EnableBasicAuth {
		t.Error("expected basic auth disabled by default")
	}
}

// --- Stop test ---

func TestBundle_Stop(t *testing.T) {
	b := &Bundle{}
	ctx := context.Background()
	if err := b.Stop(ctx); err != nil {
		t.Errorf("expected no error from Stop, got: %v", err)
	}
}

// --- Name test ---

func TestBundle_Name(t *testing.T) {
	b := NewBundle(Config{})
	if b.Name() != "prometheus" {
		t.Errorf("expected 'prometheus', got %q", b.Name())
	}
}

// --- Health check tests ---

func TestPrometheusHealthCheck_Name(t *testing.T) {
	cfg := Config{
		Namespace:        "test",
		HistogramBuckets: []float64{0.01, 0.1},
		ServiceLabels:    map[string]string{},
	}
	b := newInitializedBundle(t, cfg)
	checks := b.HealthChecks()

	if len(checks) != 1 {
		t.Fatalf("expected 1 health check, got %d", len(checks))
	}
	if checks[0].Name() != "prometheus" {
		t.Errorf("expected health check name 'prometheus', got %q", checks[0].Name())
	}
}

func TestPrometheusHealthCheck_Liveness(t *testing.T) {
	cfg := Config{
		Namespace:        "test",
		HistogramBuckets: []float64{0.01, 0.1},
		ServiceLabels:    map[string]string{},
	}
	b := newInitializedBundle(t, cfg)
	checks := b.HealthChecks()

	ctx := context.Background()
	if err := checks[0].Liveness(ctx); err != nil {
		t.Errorf("expected liveness to pass, got: %v", err)
	}
}

func TestPrometheusHealthCheck_Readiness(t *testing.T) {
	cfg := Config{
		Namespace:            "test",
		EnableDefaultMetrics: false, // avoid registering go/process collectors
		EnableHTTPMetrics:    true,
		EnableGRPCMetrics:    false,
		EnableHealthMetrics:  false,
		EnableBundleMetrics:  false,
		HistogramBuckets:     []float64{0.01, 0.1},
		ServiceLabels:        map[string]string{},
	}
	b := newInitializedBundle(t, cfg)

	// Trigger a counter so there's metric data to gather
	b.RecordHTTPRequest("GET", "/", 200, time.Millisecond)

	checks := b.HealthChecks()
	ctx := context.Background()
	if err := checks[0].Readiness(ctx); err != nil {
		t.Errorf("expected readiness to pass, got: %v", err)
	}
}

// --- isValidMetricName tests ---

func TestIsValidMetricName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"valid_name", true},
		{"valid:name", true},
		{"ValidName", true},
		{"_underscore", true},
		{"", false},
		{"123start", false},
		{"-dash", false},
	}
	for _, tt := range tests {
		result := isValidMetricName(tt.name)
		if result != tt.valid {
			t.Errorf("isValidMetricName(%q) = %v, want %v", tt.name, result, tt.valid)
		}
	}
}

// --- isValidLabelName tests ---

func TestIsValidLabelName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"valid_label", true},
		{"ValidLabel", true},
		{"label123", true},
		{"_label", true},
		{"", false},
		{"123label", false},
		{"label-name", false},
	}
	for _, tt := range tests {
		result := isValidLabelName(tt.name)
		if result != tt.valid {
			t.Errorf("isValidLabelName(%q) = %v, want %v", tt.name, result, tt.valid)
		}
	}
}

// --- StartMetricsCollection tests ---

func TestStartMetricsCollection_EmptyCollectors(t *testing.T) {
	b := &Bundle{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Should return immediately without starting goroutine
	b.StartMetricsCollection(ctx, nil, 100*time.Millisecond)
}

func TestStartMetricsCollection_ZeroInterval(t *testing.T) {
	b := &Bundle{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b.StartMetricsCollection(ctx, []MetricsCollector{}, 0)
}

// --- Registry and Gatherer accessors ---

func TestBundle_RegistryAndGatherer(t *testing.T) {
	cfg := Config{
		Namespace:        "test",
		HistogramBuckets: []float64{0.01, 0.1},
		ServiceLabels:    map[string]string{},
	}
	b := newInitializedBundle(t, cfg)

	if b.Registry() == nil {
		t.Error("expected non-nil registry")
	}
	if b.Gatherer() == nil {
		t.Error("expected non-nil gatherer")
	}
}
