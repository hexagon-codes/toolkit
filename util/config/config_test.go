package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	c := New()
	if c == nil {
		t.Fatal("New() should return non-nil config")
	}
}

func TestSetGet(t *testing.T) {
	c := New()

	c.Set("key", "value")
	v, ok := c.Get("key")

	if !ok {
		t.Error("Get should return true for existing key")
	}
	if v != "value" {
		t.Errorf("expected 'value', got %v", v)
	}
}

func TestGetNotFound(t *testing.T) {
	c := New()

	_, ok := c.Get("missing")
	if ok {
		t.Error("Get should return false for missing key")
	}
}

func TestGetString(t *testing.T) {
	c := New()
	c.Set("name", "Alice")

	name := c.GetString("name")
	if name != "Alice" {
		t.Errorf("expected 'Alice', got '%s'", name)
	}

	// Missing key
	if c.GetString("missing") != "" {
		t.Error("GetString should return empty string for missing key")
	}
}

func TestGetStringDefault(t *testing.T) {
	c := New()
	c.Set("name", "Alice")

	// Existing key
	name := c.GetStringDefault("name", "default")
	if name != "Alice" {
		t.Errorf("expected 'Alice', got '%s'", name)
	}

	// Missing key
	name = c.GetStringDefault("missing", "default")
	if name != "default" {
		t.Errorf("expected 'default', got '%s'", name)
	}
}

func TestGetInt(t *testing.T) {
	c := New()
	c.Set("count", int64(42))

	count := c.GetInt("count")
	if count != 42 {
		t.Errorf("expected 42, got %d", count)
	}

	// From string
	c.Set("port", "8080")
	port := c.GetInt("port")
	if port != 8080 {
		t.Errorf("expected 8080, got %d", port)
	}

	// Missing key
	if c.GetInt("missing") != 0 {
		t.Error("GetInt should return 0 for missing key")
	}
}

func TestGetIntDefault(t *testing.T) {
	c := New()
	c.Set("count", int64(42))

	// Existing key
	count := c.GetIntDefault("count", 100)
	if count != 42 {
		t.Errorf("expected 42, got %d", count)
	}

	// Missing key
	count = c.GetIntDefault("missing", 100)
	if count != 100 {
		t.Errorf("expected 100, got %d", count)
	}
}

func TestGetInt64(t *testing.T) {
	c := New()
	c.Set("big", int64(9223372036854775807))

	big := c.GetInt64("big")
	if big != 9223372036854775807 {
		t.Errorf("expected max int64, got %d", big)
	}
}

func TestGetFloat64(t *testing.T) {
	c := New()
	c.Set("pi", 3.14)

	pi := c.GetFloat64("pi")
	if pi != 3.14 {
		t.Errorf("expected 3.14, got %f", pi)
	}

	// From string
	c.Set("rate", "0.5")
	rate := c.GetFloat64("rate")
	if rate != 0.5 {
		t.Errorf("expected 0.5, got %f", rate)
	}
}

func TestGetFloat64Default(t *testing.T) {
	c := New()

	rate := c.GetFloat64Default("rate", 1.0)
	if rate != 1.0 {
		t.Errorf("expected 1.0, got %f", rate)
	}
}

func TestGetBool(t *testing.T) {
	c := New()
	c.Set("enabled", true)

	if !c.GetBool("enabled") {
		t.Error("expected true")
	}

	// From string
	c.Set("debug", "true")
	if !c.GetBool("debug") {
		t.Error("expected true from string")
	}

	c.Set("verbose", "1")
	if !c.GetBool("verbose") {
		t.Error("expected true from '1'")
	}

	c.Set("yes_flag", "yes")
	if !c.GetBool("yes_flag") {
		t.Error("expected true from 'yes'")
	}

	// Missing key
	if c.GetBool("missing") {
		t.Error("GetBool should return false for missing key")
	}
}

func TestGetBoolDefault(t *testing.T) {
	c := New()

	// Missing key
	if !c.GetBoolDefault("missing", true) {
		t.Error("expected default true")
	}
}

func TestGetDuration(t *testing.T) {
	c := New()
	c.Set("timeout", time.Second*30)

	timeout := c.GetDuration("timeout")
	if timeout != 30*time.Second {
		t.Errorf("expected 30s, got %v", timeout)
	}

	// From string
	c.Set("interval", "5m")
	interval := c.GetDuration("interval")
	if interval != 5*time.Minute {
		t.Errorf("expected 5m, got %v", interval)
	}
}

func TestGetDurationDefault(t *testing.T) {
	c := New()

	timeout := c.GetDurationDefault("timeout", 10*time.Second)
	if timeout != 10*time.Second {
		t.Errorf("expected 10s, got %v", timeout)
	}
}

func TestGetStringSlice(t *testing.T) {
	c := New()

	// From []string
	c.Set("hosts", []string{"host1", "host2"})
	hosts := c.GetStringSlice("hosts")
	if len(hosts) != 2 {
		t.Errorf("expected 2 hosts, got %d", len(hosts))
	}

	// From comma-separated string
	c.Set("ports", "8080, 8081, 8082")
	ports := c.GetStringSlice("ports")
	if len(ports) != 3 {
		t.Errorf("expected 3 ports, got %d", len(ports))
	}
	if ports[0] != "8080" {
		t.Errorf("expected '8080', got '%s'", ports[0])
	}
}

func TestGetStringMap(t *testing.T) {
	c := New()

	m := map[string]any{
		"host": "localhost",
		"port": "5432",
	}
	c.Set("database", m)

	result := c.GetStringMap("database")
	if result["host"] != "localhost" {
		t.Errorf("expected 'localhost', got '%s'", result["host"])
	}
}

func TestHas(t *testing.T) {
	c := New()
	c.Set("key", "value")

	if !c.Has("key") {
		t.Error("Has should return true for existing key")
	}
	if c.Has("missing") {
		t.Error("Has should return false for missing key")
	}
}

func TestKeys(t *testing.T) {
	c := New()
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)

	keys := c.Keys()
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}
}

func TestAll(t *testing.T) {
	c := New()
	c.Set("a", 1)
	c.Set("b", 2)

	all := c.All()
	if len(all) != 2 {
		t.Errorf("expected 2 items, got %d", len(all))
	}
}

func TestLoadJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	content := `{
		"name": "test",
		"port": 8080,
		"enabled": true
	}`
	os.WriteFile(path, []byte(content), 0644)

	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if c.GetString("name") != "test" {
		t.Error("name should be 'test'")
	}
	if c.GetInt("port") != 8080 {
		t.Error("port should be 8080")
	}
	if !c.GetBool("enabled") {
		t.Error("enabled should be true")
	}
}

func TestLoadYAML(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")

	content := `
name: test
port: 8080
enabled: true
timeout: 30s
`
	os.WriteFile(path, []byte(content), 0644)

	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if c.GetString("name") != "test" {
		t.Errorf("name should be 'test', got '%s'", c.GetString("name"))
	}
}

func TestLoadTOML(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.toml")

	content := `
name = "test"
port = 8080

[database]
host = "localhost"
`
	os.WriteFile(path, []byte(content), 0644)

	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if c.GetString("name") != "test" {
		t.Errorf("name should be 'test', got '%s'", c.GetString("name"))
	}
	if c.GetString("database.host") != "localhost" {
		t.Errorf("database.host should be 'localhost', got '%s'", c.GetString("database.host"))
	}
}

func TestLoadEnvFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, ".env")

	content := `
APP_NAME=test
APP_PORT=8080
# This is a comment
APP_DEBUG=true
`
	os.WriteFile(path, []byte(content), 0644)

	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if c.GetString("APP_NAME") != "test" {
		t.Errorf("APP_NAME should be 'test', got '%s'", c.GetString("APP_NAME"))
	}
}

func TestLoadEnv(t *testing.T) {
	// Set environment variable
	os.Setenv("TEST_APP_NAME", "myapp")
	os.Setenv("TEST_APP_PORT", "9090")
	defer os.Unsetenv("TEST_APP_NAME")
	defer os.Unsetenv("TEST_APP_PORT")

	c := New()
	c.LoadEnv("TEST_APP")

	if c.GetString("name") != "myapp" {
		t.Errorf("name should be 'myapp', got '%s'", c.GetString("name"))
	}
	if c.GetInt("port") != 9090 {
		t.Errorf("port should be 9090, got %d", c.GetInt("port"))
	}
}

func TestGetFromEnv(t *testing.T) {
	os.Setenv("DATABASE_HOST", "localhost")
	defer os.Unsetenv("DATABASE_HOST")

	c := New()

	// Should find from environment variable
	host := c.GetString("database.host")
	if host != "localhost" {
		t.Errorf("expected 'localhost', got '%s'", host)
	}
}

func TestUnmarshal(t *testing.T) {
	c := New()
	c.Set("name", "test")
	c.Set("port", int64(8080))

	var cfg struct {
		Name string `json:"name"`
		Port int    `json:"port"`
	}

	err := c.Unmarshal(&cfg)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if cfg.Name != "test" {
		t.Errorf("expected 'test', got '%s'", cfg.Name)
	}
	if cfg.Port != 8080 {
		t.Errorf("expected 8080, got %d", cfg.Port)
	}
}

func TestUnmarshalKey(t *testing.T) {
	c := New()
	c.Set("database", map[string]any{
		"host": "localhost",
		"port": float64(5432),
	})

	var db struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}

	err := c.UnmarshalKey("database", &db)
	if err != nil {
		t.Fatalf("UnmarshalKey failed: %v", err)
	}

	if db.Host != "localhost" {
		t.Errorf("expected 'localhost', got '%s'", db.Host)
	}
}

func TestUnmarshalKeyNotFound(t *testing.T) {
	c := New()

	var db struct{}
	err := c.UnmarshalKey("missing", &db)

	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestBindEnv(t *testing.T) {
	os.Setenv("APP_NAME", "myapp")
	os.Setenv("APP_PORT", "8080")
	os.Setenv("APP_DEBUG", "true")
	os.Setenv("APP_TIMEOUT", "30s")
	defer func() {
		os.Unsetenv("APP_NAME")
		os.Unsetenv("APP_PORT")
		os.Unsetenv("APP_DEBUG")
		os.Unsetenv("APP_TIMEOUT")
	}()

	var cfg struct {
		Name    string        `env:"NAME"`
		Port    int           `env:"PORT"`
		Debug   bool          `env:"DEBUG"`
		Timeout time.Duration `env:"TIMEOUT"`
	}

	err := BindEnv(&cfg, "APP")
	if err != nil {
		t.Fatalf("BindEnv failed: %v", err)
	}

	if cfg.Name != "myapp" {
		t.Errorf("expected 'myapp', got '%s'", cfg.Name)
	}
	if cfg.Port != 8080 {
		t.Errorf("expected 8080, got %d", cfg.Port)
	}
	if !cfg.Debug {
		t.Error("expected Debug to be true")
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("expected 30s, got %v", cfg.Timeout)
	}
}

func TestBindEnvWithDefault(t *testing.T) {
	var cfg struct {
		Port int `env:"MISSING_PORT" default:"3000"`
	}

	err := BindEnv(&cfg, "")
	if err != nil {
		t.Fatalf("BindEnv failed: %v", err)
	}

	if cfg.Port != 3000 {
		t.Errorf("expected default 3000, got %d", cfg.Port)
	}
}

func TestBindEnvInvalidType(t *testing.T) {
	var notPtr struct{}
	err := BindEnv(notPtr, "")

	if err != ErrInvalidType {
		t.Errorf("expected ErrInvalidType, got %v", err)
	}
}

func TestBindEnvSlice(t *testing.T) {
	os.Setenv("APP_HOSTS", "host1,host2,host3")
	defer os.Unsetenv("APP_HOSTS")

	var cfg struct {
		Hosts []string `env:"HOSTS"`
	}

	err := BindEnv(&cfg, "APP")
	if err != nil {
		t.Fatalf("BindEnv failed: %v", err)
	}

	if len(cfg.Hosts) != 3 {
		t.Errorf("expected 3 hosts, got %d", len(cfg.Hosts))
	}
}

func TestBindEnvUint(t *testing.T) {
	os.Setenv("APP_COUNT", "100")
	defer os.Unsetenv("APP_COUNT")

	var cfg struct {
		Count uint `env:"COUNT"`
	}

	err := BindEnv(&cfg, "APP")
	if err != nil {
		t.Fatalf("BindEnv failed: %v", err)
	}

	if cfg.Count != 100 {
		t.Errorf("expected 100, got %d", cfg.Count)
	}
}

func TestBindEnvFloat(t *testing.T) {
	os.Setenv("APP_RATE", "0.5")
	defer os.Unsetenv("APP_RATE")

	var cfg struct {
		Rate float64 `env:"RATE"`
	}

	err := BindEnv(&cfg, "APP")
	if err != nil {
		t.Fatalf("BindEnv failed: %v", err)
	}

	if cfg.Rate != 0.5 {
		t.Errorf("expected 0.5, got %f", cfg.Rate)
	}
}

func TestGlobalConfig(t *testing.T) {
	c := New()
	c.Set("key", "value")
	SetGlobal(c)

	v, ok := Get("key")
	if !ok || v != "value" {
		t.Error("Global config not working")
	}

	if GetString("key") != "value" {
		t.Error("GetString from global not working")
	}
}

func TestLoadUnsupportedFormat(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.xyz")
	os.WriteFile(path, []byte("data"), 0644)

	_, err := Load(path)
	if err != ErrUnsupportedFormat {
		t.Errorf("expected ErrUnsupportedFormat, got %v", err)
	}
}

func TestLoadNonExistentFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.json")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestParseValue(t *testing.T) {
	tests := []struct {
		input    string
		expected any
	}{
		{"true", true},
		{"false", false},
		{"42", int64(42)},
		{"3.14", 3.14},
		{"hello", "hello"},
	}

	for _, tt := range tests {
		result := parseValue(tt.input)
		switch expected := tt.expected.(type) {
		case int64:
			if r, ok := result.(int64); !ok || r != expected {
				t.Errorf("parseValue(%s) = %v, want %v", tt.input, result, expected)
			}
		case float64:
			if r, ok := result.(float64); !ok || r != expected {
				t.Errorf("parseValue(%s) = %v, want %v", tt.input, result, expected)
			}
		case bool:
			if r, ok := result.(bool); !ok || r != expected {
				t.Errorf("parseValue(%s) = %v, want %v", tt.input, result, expected)
			}
		case string:
			if r, ok := result.(string); !ok || r != expected {
				t.Errorf("parseValue(%s) = %v, want %v", tt.input, result, expected)
			}
		}
	}
}

func TestGlobalFunctions(t *testing.T) {
	c := New()
	c.Set("str", "value")
	c.Set("num", int64(42))
	c.Set("flag", true)
	c.Set("dur", time.Second)
	SetGlobal(c)

	if GetStringDefault("missing", "default") != "default" {
		t.Error("GetStringDefault failed")
	}

	if GetInt("num") != 42 {
		t.Error("GetInt failed")
	}

	if GetIntDefault("missing", 100) != 100 {
		t.Error("GetIntDefault failed")
	}

	if !GetBool("flag") {
		t.Error("GetBool failed")
	}

	if GetDuration("dur") != time.Second {
		t.Error("GetDuration failed")
	}

	Set("new", "value")
	if !Has("new") {
		t.Error("Set/Has failed")
	}
}

func TestLoadGlobal(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	content := `{"name": "global"}`
	os.WriteFile(path, []byte(content), 0644)

	err := LoadGlobal(path)
	if err != nil {
		t.Fatalf("LoadGlobal failed: %v", err)
	}

	if GetString("name") != "global" {
		t.Error("Global config not loaded")
	}
}

func TestLoadGlobalError(t *testing.T) {
	err := LoadGlobal("/nonexistent/config.json")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestYAMLWithComments(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yml")

	content := `# Comment line
name: test
# Another comment
port: 8080
`
	os.WriteFile(path, []byte(content), 0644)

	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if c.GetString("name") != "test" {
		t.Errorf("name should be 'test', got '%s'", c.GetString("name"))
	}
}

func TestYAMLWithQuotes(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")

	content := `
name: "test value"
path: '/usr/local'
`
	os.WriteFile(path, []byte(content), 0644)

	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if c.GetString("name") != "test value" {
		t.Errorf("name should be 'test value', got '%s'", c.GetString("name"))
	}
	if c.GetString("path") != "/usr/local" {
		t.Errorf("path should be '/usr/local', got '%s'", c.GetString("path"))
	}
}

func BenchmarkGet(b *testing.B) {
	c := New()
	c.Set("key", "value")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get("key")
	}
}

func BenchmarkGetString(b *testing.B) {
	c := New()
	c.Set("key", "value")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.GetString("key")
	}
}
