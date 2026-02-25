package configloader

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// loaderWithConfig builds a Loader with the given Config for unit testing.
func loaderWithConfig(cfg Config) *Loader {
	return &Loader{config: cfg}
}

// TestValidateFilePath covers path acceptance / rejection rules.
func TestValidateFilePath(t *testing.T) {
	l := loaderWithConfig(DefaultConfig())

	t.Run("relative path accepted", func(t *testing.T) {
		// ./config.yaml is the canonical DefaultConfig path; it should be accepted.
		if err := l.validateFilePath("./config.yaml"); err != nil {
			t.Errorf("expected relative path to be accepted, got error: %v", err)
		}
	})

	t.Run("absolute path accepted", func(t *testing.T) {
		absPath := "/etc/forge/config.yaml"
		if err := l.validateFilePath(absPath); err != nil {
			t.Errorf("expected absolute path to be accepted, got error: %v", err)
		}
	})

	t.Run("empty path rejected", func(t *testing.T) {
		if err := l.validateFilePath(""); err == nil {
			t.Error("expected empty path to be rejected, got nil error")
		}
	})

	t.Run("path traversal rejected", func(t *testing.T) {
		// A raw traversal sequence that escapes the working directory.
		if err := l.validateFilePath("../../../etc/passwd"); err == nil {
			t.Error("expected path traversal to be rejected, got nil error")
		}
	})
}

// TestValidateFilePathAllowedPaths verifies that AllowedPaths restriction is enforced.
func TestValidateFilePathAllowedPaths(t *testing.T) {
	dir := t.TempDir()

	l := loaderWithConfig(Config{
		ConfigPaths:  []string{filepath.Join(dir, "config.yaml")},
		AllowedPaths: []string{dir},
		MaxFileSize:  1024 * 1024,
	})

	t.Run("path within allowed directory accepted", func(t *testing.T) {
		allowed := filepath.Join(dir, "config.yaml")
		if err := l.validateFilePath(allowed); err != nil {
			t.Errorf("expected path within allowed dir to be accepted, got: %v", err)
		}
	})

	t.Run("path outside allowed directory rejected", func(t *testing.T) {
		outside := "/tmp/secret.yaml"
		if err := l.validateFilePath(outside); err == nil {
			t.Error("expected path outside allowed dir to be rejected, got nil error")
		}
	})
}

// TestDefaultConfigPathsPassValidation ensures every path in DefaultConfig passes validateFilePath.
func TestDefaultConfigPathsPassValidation(t *testing.T) {
	l := loaderWithConfig(DefaultConfig())

	for _, path := range DefaultConfig().ConfigPaths {
		path := path // capture
		t.Run(path, func(t *testing.T) {
			if err := l.validateFilePath(path); err != nil {
				t.Errorf("DefaultConfig path %q failed validateFilePath: %v", path, err)
			}
		})
	}
}

// TestConfigValidate checks the Config.Validate method.
func TestConfigValidate(t *testing.T) {
	t.Run("default config is valid", func(t *testing.T) {
		cfg := DefaultConfig()
		if err := cfg.Validate(); err != nil {
			t.Errorf("DefaultConfig should be valid, got: %v", err)
		}
	})

	t.Run("empty ConfigPaths rejected", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.ConfigPaths = nil
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for empty ConfigPaths, got nil")
		}
	})

	t.Run("empty string path rejected", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.ConfigPaths = []string{""}
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for empty string path, got nil")
		}
	})

	t.Run("relative path without ./ prefix rejected by Validate", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.ConfigPaths = []string{"config.yaml"} // no ./ prefix
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for relative path without ./ prefix, got nil")
		}
	})
}

// TestLoaderLoad_DefaultsApplied tests that defaults from struct tags are applied.
func TestLoaderLoad_DefaultsApplied(t *testing.T) {
	type TestCfg struct {
		Name    string `default:"world"`
		Count   int    `default:"42"`
		Enabled bool   `default:"true"`
	}

	l := loaderWithConfig(Config{
		ConfigPaths:    []string{"./nonexistent.yaml"},
		RequireConfigFile: false,
		ValidateOnLoad: false,
		MaxFileSize:    1024 * 1024,
	})

	var cfg TestCfg
	result, err := l.Load(&cfg)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Name != "world" {
		t.Errorf("expected default Name='world', got %q", cfg.Name)
	}
	if cfg.Count != 42 {
		t.Errorf("expected default Count=42, got %d", cfg.Count)
	}
	if !cfg.Enabled {
		t.Error("expected default Enabled=true")
	}
	if len(result.DefaultsApplied) == 0 {
		t.Error("expected DefaultsApplied to be populated")
	}
}

// TestLoaderLoad_EnvVarOverride tests that environment variables override defaults.
func TestLoaderLoad_EnvVarOverride(t *testing.T) {
	type TestCfg struct {
		ServiceURL string `env:"TEST_SERVICE_URL_CONFIGLOADER" default:"http://localhost:8080"`
	}

	t.Setenv("TEST_SERVICE_URL_CONFIGLOADER", "http://production:9090")

	l := loaderWithConfig(Config{
		ConfigPaths:    []string{"./nonexistent.yaml"},
		RequireConfigFile: false,
		ValidateOnLoad: false,
		MaxFileSize:    1024 * 1024,
	})

	var cfg TestCfg
	result, err := l.Load(&cfg)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.ServiceURL != "http://production:9090" {
		t.Errorf("expected env override, got %q", cfg.ServiceURL)
	}
	if len(result.EnvVarsUsed) == 0 {
		t.Error("expected EnvVarsUsed to be populated")
	}
}

// TestLoaderLoad_NilDest tests that Load rejects nil destination.
func TestLoaderLoad_NilDest(t *testing.T) {
	l := loaderWithConfig(DefaultConfig())
	_, err := l.Load(nil)
	if err == nil {
		t.Error("expected error for nil destination")
	}
}

// TestLoaderLoad_NonPointerDest tests that Load rejects non-pointer destination.
func TestLoaderLoad_NonPointerDest(t *testing.T) {
	l := loaderWithConfig(DefaultConfig())
	var cfg struct{ Name string }
	_, err := l.Load(cfg) // not a pointer
	if err == nil {
		t.Error("expected error for non-pointer destination")
	}
}

// TestLoaderLoad_FromYAMLFile tests loading from a YAML file.
func TestLoaderLoad_FromYAMLFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := dir + "/config.yaml"

	content := "service_name: my-svc\ndebug: true\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	type TestCfg struct {
		ServiceName string `yaml:"service_name"`
		Debug       bool   `yaml:"debug"`
	}

	l := loaderWithConfig(Config{
		ConfigPaths:    []string{cfgPath},
		RequireConfigFile: true,
		ValidateOnLoad: false,
		MaxFileSize:    1024 * 1024,
		AllowedPaths:   []string{dir},
	})

	var cfg TestCfg
	result, err := l.Load(&cfg)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.ServiceName != "my-svc" {
		t.Errorf("expected service_name='my-svc', got %q", cfg.ServiceName)
	}
	if !cfg.Debug {
		t.Error("expected debug=true")
	}
	if result.LoadedFrom != cfgPath {
		t.Errorf("expected LoadedFrom=%q, got %q", cfgPath, result.LoadedFrom)
	}
}

// TestLoaderLoad_FromJSONFile tests loading from a JSON file.
func TestLoaderLoad_FromJSONFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := dir + "/config.json"

	content := `{"service_name":"json-svc","port":8080}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	type TestCfg struct {
		ServiceName string `json:"service_name"`
		Port        int    `json:"port"`
	}

	l := loaderWithConfig(Config{
		ConfigPaths:    []string{cfgPath},
		RequireConfigFile: true,
		ValidateOnLoad: false,
		MaxFileSize:    1024 * 1024,
		AllowedPaths:   []string{dir},
	})

	var cfg TestCfg
	_, err := l.Load(&cfg)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.ServiceName != "json-svc" {
		t.Errorf("expected service_name='json-svc', got %q", cfg.ServiceName)
	}
	if cfg.Port != 8080 {
		t.Errorf("expected port=8080, got %d", cfg.Port)
	}
}

// TestLoaderLoad_RequireConfigFile_NotFound tests error when required file missing.
func TestLoaderLoad_RequireConfigFile_NotFound(t *testing.T) {
	l := loaderWithConfig(Config{
		ConfigPaths:    []string{"./definitely_does_not_exist_xyz.yaml"},
		RequireConfigFile: true,
		ValidateOnLoad: false,
		MaxFileSize:    1024 * 1024,
	})

	var cfg struct{ Name string }
	_, err := l.Load(&cfg)
	if err == nil {
		t.Error("expected error for required missing file")
	}
}

// TestLoaderLoad_FileTooLarge tests that oversized files are rejected.
func TestLoaderLoad_FileTooLarge(t *testing.T) {
	dir := t.TempDir()
	cfgPath := dir + "/config.yaml"

	// Write 100 bytes but set limit to 10
	if err := os.WriteFile(cfgPath, []byte("key: value\nother: something\nmore: data\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	l := loaderWithConfig(Config{
		ConfigPaths:    []string{cfgPath},
		RequireConfigFile: false,
		ValidateOnLoad: false,
		MaxFileSize:    5, // very small
		AllowedPaths:   []string{dir},
	})

	var cfg struct{ Key string `yaml:"key"` }
	_, err := l.Load(&cfg)
	if err == nil {
		t.Error("expected error for oversized config file")
	}
}

// TestLoaderLoadFromString_YAML tests LoadFromString with YAML format.
func TestLoaderLoadFromString_YAML(t *testing.T) {
	l := loaderWithConfig(DefaultConfig())

	type TestCfg struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	}

	var cfg TestCfg
	err := l.LoadFromString("host: localhost\nport: 5432\n", "yaml", &cfg)
	if err != nil {
		t.Fatalf("LoadFromString failed: %v", err)
	}
	if cfg.Host != "localhost" {
		t.Errorf("expected host='localhost', got %q", cfg.Host)
	}
	if cfg.Port != 5432 {
		t.Errorf("expected port=5432, got %d", cfg.Port)
	}
}

// TestLoaderLoadFromString_JSON tests LoadFromString with JSON format.
func TestLoaderLoadFromString_JSON(t *testing.T) {
	l := loaderWithConfig(DefaultConfig())

	type TestCfg struct {
		Host string `json:"host"`
	}

	var cfg TestCfg
	err := l.LoadFromString(`{"host":"prod-db"}`, "json", &cfg)
	if err != nil {
		t.Fatalf("LoadFromString failed: %v", err)
	}
	if cfg.Host != "prod-db" {
		t.Errorf("expected host='prod-db', got %q", cfg.Host)
	}
}

// TestLoaderLoadFromString_UnsupportedFormat tests that unknown formats are rejected.
func TestLoaderLoadFromString_UnsupportedFormat(t *testing.T) {
	l := loaderWithConfig(DefaultConfig())
	var cfg struct{}
	if err := l.LoadFromString("data", "toml", &cfg); err == nil {
		t.Error("expected error for unsupported format")
	}
}

// TestLoaderGetConfigInfo tests the GetConfigInfo method.
func TestLoaderGetConfigInfo(t *testing.T) {
	l := loaderWithConfig(DefaultConfig())
	info := l.GetConfigInfo()

	if info["config_paths"] == nil {
		t.Error("expected config_paths in info")
	}
	if info["validate_on_load"] == nil {
		t.Error("expected validate_on_load in info")
	}
}

// TestLoaderOnConfigChange tests callback registration.
func TestLoaderOnConfigChange(t *testing.T) {
	l := loaderWithConfig(DefaultConfig())
	called := 0
	l.OnConfigChange(func(cfg interface{}) {
		called++
	})
	if len(l.changeCallbacks) != 1 {
		t.Errorf("expected 1 callback, got %d", len(l.changeCallbacks))
	}
}

// TestToEnvVarName tests the env var name conversion utility.
func TestToEnvVarName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ServiceName", "SERVICE_NAME"},
		{"Debug", "DEBUG"},
		{"HTTPAddr", "H_T_T_P_ADDR"},
		{"Port", "PORT"},
	}
	for _, tt := range tests {
		result := toEnvVarName(tt.input)
		if result != tt.expected {
			t.Errorf("toEnvVarName(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// TestSanitizeValue tests sensitive data redaction.
func TestSanitizeValue(t *testing.T) {
	l := loaderWithConfig(Config{SecureLogging: true, MaxFileSize: 1024 * 1024})

	type TestCfg struct {
		Password string `sensitive:"true"`
		Normal   string
		APIKey   string `env:"API_KEY"`
	}

	cfgType := reflect.TypeOf(TestCfg{})

	// sensitive tag
	field, _ := cfgType.FieldByName("Password")
	if got := l.sanitizeValue(field, "mysecret"); got != "[REDACTED]" {
		t.Errorf("expected [REDACTED] for sensitive field, got %q", got)
	}

	// normal field
	field, _ = cfgType.FieldByName("Normal")
	if got := l.sanitizeValue(field, "hello"); got != "hello" {
		t.Errorf("expected 'hello' for normal field, got %q", got)
	}

	// env tag with sensitive keyword
	field, _ = cfgType.FieldByName("APIKey")
	if got := l.sanitizeValue(field, "abc123"); got != "[REDACTED]" {
		t.Errorf("expected [REDACTED] for env tag with 'key', got %q", got)
	}
}

// TestSetFieldValue tests field value assignment for various types.
func TestSetFieldValue(t *testing.T) {
	l := loaderWithConfig(DefaultConfig())

	type TestCfg struct {
		StrField    string
		IntField    int
		BoolField   bool
		FloatField  float64
		UintField   uint
		SliceField  []string
	}

	cfg := TestCfg{}
	v := reflect.ValueOf(&cfg).Elem()

	tests := []struct {
		fieldName string
		value     string
		checkFn   func() bool
	}{
		{"StrField", "hello", func() bool { return cfg.StrField == "hello" }},
		{"IntField", "42", func() bool { return cfg.IntField == 42 }},
		{"BoolField", "true", func() bool { return cfg.BoolField == true }},
		{"FloatField", "3.14", func() bool { return cfg.FloatField == 3.14 }},
		{"UintField", "7", func() bool { return cfg.UintField == 7 }},
		{"SliceField", "a,b,c", func() bool { return len(cfg.SliceField) == 3 && cfg.SliceField[0] == "a" }},
	}

	for _, tt := range tests {
		field := v.FieldByName(tt.fieldName)
		if err := l.setFieldValue(field, tt.value); err != nil {
			t.Errorf("setFieldValue(%s, %q) error: %v", tt.fieldName, tt.value, err)
			continue
		}
		if !tt.checkFn() {
			t.Errorf("setFieldValue(%s, %q) did not set expected value", tt.fieldName, tt.value)
		}
	}
}

// TestSetFieldValue_InvalidBool tests invalid boolean parsing.
func TestSetFieldValue_InvalidBool(t *testing.T) {
	l := loaderWithConfig(DefaultConfig())
	type Cfg struct{ B bool }
	cfg := Cfg{}
	field := reflect.ValueOf(&cfg).Elem().FieldByName("B")
	if err := l.setFieldValue(field, "notabool"); err == nil {
		t.Error("expected error for invalid bool")
	}
}

// TestSetFieldValue_InvalidInt tests invalid integer parsing.
func TestSetFieldValue_InvalidInt(t *testing.T) {
	l := loaderWithConfig(DefaultConfig())
	type Cfg struct{ N int }
	cfg := Cfg{}
	field := reflect.ValueOf(&cfg).Elem().FieldByName("N")
	if err := l.setFieldValue(field, "notanint"); err == nil {
		t.Error("expected error for invalid int")
	}
}

// TestBundle_Name tests the bundle name accessor.
func TestBundle_Name(t *testing.T) {
	b := NewBundle(DefaultConfig())
	if b.Name() != "config-loader" {
		t.Errorf("expected 'config-loader', got %q", b.Name())
	}
}

// TestBundle_Initialize tests bundle initialization.
func TestBundle_Initialize(t *testing.T) {
	cfg := DefaultConfig()
	b := NewBundle(cfg)
	if err := b.Initialize(nil); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if b.Loader() == nil {
		t.Error("expected non-nil loader after Initialize")
	}
}

// TestBundle_Stop_NoWatcher tests stop with no watcher.
func TestBundle_Stop_NoWatcher(t *testing.T) {
	b := NewBundle(DefaultConfig())
	if err := b.Stop(context.Background()); err != nil {
		t.Errorf("expected no error for Stop without watcher, got: %v", err)
	}
}

// TestBundle_Close tests the deprecated Close method.
func TestBundle_Close(t *testing.T) {
	b := NewBundle(DefaultConfig())
	if err := b.Close(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestLoaderMustLoad_Panics tests that MustLoad panics on error.
func TestLoaderMustLoad_Panics(t *testing.T) {
	l := loaderWithConfig(Config{
		ConfigPaths:    []string{"./no_such_file.yaml"},
		RequireConfigFile: true,
		ValidateOnLoad: false,
		MaxFileSize:    1024,
	})

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected MustLoad to panic on error")
		}
	}()

	var cfg struct{ Name string }
	l.MustLoad(&cfg)
}

// TestLoaderReload tests that Reload re-reads configuration.
func TestLoaderReload(t *testing.T) {
	dir := t.TempDir()
	cfgPath := dir + "/config.yaml"

	type TestCfg struct {
		Value string `yaml:"value"`
	}

	if err := os.WriteFile(cfgPath, []byte("value: initial\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	l := loaderWithConfig(Config{
		ConfigPaths:    []string{cfgPath},
		RequireConfigFile: false,
		ValidateOnLoad: false,
		MaxFileSize:    1024 * 1024,
		AllowedPaths:   []string{dir},
	})

	var cfg TestCfg
	if _, err := l.Load(&cfg); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Value != "initial" {
		t.Errorf("expected 'initial', got %q", cfg.Value)
	}

	// Update file
	if err := os.WriteFile(cfgPath, []byte("value: updated\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := l.Reload(&cfg); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if cfg.Value != "updated" {
		t.Errorf("expected 'updated', got %q", cfg.Value)
	}
}

// TestLooksLikeSensitiveData tests sensitive data detection heuristics.
func TestLooksLikeSensitiveData(t *testing.T) {
	l := loaderWithConfig(Config{SecureLogging: true, MaxFileSize: 1024 * 1024})

	// Hex hash (32+ chars)
	hexValue := "abcdef1234567890abcdef1234567890"
	if !l.looksLikeSensitiveData(hexValue) {
		t.Errorf("expected hex value to be flagged as sensitive")
	}

	// Short normal value
	if l.looksLikeSensitiveData("hello") {
		t.Error("expected short normal value to not be flagged")
	}

	// Empty value
	if l.looksLikeSensitiveData("") {
		t.Error("expected empty value to not be flagged")
	}
}

// TestLoadConfig_Convenience tests the LoadConfig generic helper.
func TestLoadConfig_Convenience(t *testing.T) {
	dir := t.TempDir()
	cfgPath := dir + "/config.yaml"

	type TestCfg struct {
		AppName string `yaml:"app_name"`
	}

	if err := os.WriteFile(cfgPath, []byte("app_name: testapp\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, result, err := LoadConfig[TestCfg](cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.AppName != "testapp" {
		t.Errorf("expected app_name='testapp', got %q", cfg.AppName)
	}
	if result == nil {
		t.Error("expected non-nil result")
	}
}

// TestLoadFromFile_RelativePath verifies the full loadFromFile code path works with
// a relative path pointing at a real YAML file.
func TestLoadFromFile_RelativePath(t *testing.T) {
	// Write a temporary YAML file in the current working directory so
	// a relative reference to it resolves correctly.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}

	tmp, err := os.CreateTemp(cwd, "forge-test-*.yaml")
	if err != nil {
		t.Fatalf("os.CreateTemp: %v", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.WriteString("key: value\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	tmp.Close()

	// Chmod to 0644 to satisfy RequiredFileMode default.
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	// Use only the base name (relative to cwd) to exercise relative-path resolution.
	relPath := "./" + filepath.Base(tmp.Name())

	l := loaderWithConfig(DefaultConfig())

	var dest map[string]interface{}
	if err := l.loadFromFile(relPath, &dest); err != nil {
		t.Errorf("loadFromFile with relative path failed: %v", err)
	}
}

func TestMustLoadConfig_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected MustLoadConfig to panic on nonexistent required file")
		}
	}()
	// Force a config with a required file that doesn't exist
	// Use a path that definitely doesn't exist
	MustLoadConfig[map[string]interface{}]("/nonexistent/path/that/does/not/exist.yaml")
}

func TestLooksLikeSensitiveData_JWTToken(t *testing.T) {
	l := loaderWithConfig(DefaultConfig())
	// JWT-like token: three base64 parts separated by dots
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ0ZXN0In0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	if !l.looksLikeSensitiveData(jwt) {
		t.Error("expected JWT token to look like sensitive data")
	}
}

func TestLooksLikeSensitiveData_LongAlphanumeric(t *testing.T) {
	l := loaderWithConfig(DefaultConfig())
	// Long alphanumeric string (>40 chars) — looks like API key
	key := "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLM"
	if !l.looksLikeSensitiveData(key) {
		t.Error("expected long alphanumeric string to look like sensitive data")
	}
}

func TestLooksLikeSensitiveData_ShortString(t *testing.T) {
	l := loaderWithConfig(DefaultConfig())
	if l.looksLikeSensitiveData("hello") {
		t.Error("expected short string to not look like sensitive data")
	}
}

func TestLooksLikeSensitiveData_EmptyString(t *testing.T) {
	l := loaderWithConfig(DefaultConfig())
	if l.looksLikeSensitiveData("") {
		t.Error("expected empty string to not look like sensitive data")
	}
}
