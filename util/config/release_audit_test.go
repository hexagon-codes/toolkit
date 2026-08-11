package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func writeAuditConfig(t *testing.T, name, content string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod config fixture: %v", err)
	}
	return path
}

func TestLoadFileReplacesSnapshotAndPreservesItOnParseFailure(t *testing.T) {
	c := New()
	c.Set("old", "value")

	validPath := writeAuditConfig(t, "config.json", `{"new":1}`, 0o600)
	if err := c.LoadFile(validPath); err != nil {
		t.Fatalf("load valid config: %v", err)
	}
	if c.Has("old") {
		t.Fatal("successful reload retained a key from the previous snapshot")
	}
	if got := c.GetInt64("new"); got != 1 {
		t.Fatalf("new value = %d, want 1", got)
	}

	before := c.All()
	invalidPath := writeAuditConfig(t, "config.yaml", "partial: value\ninvalid line\n", 0o600)
	if err := c.LoadFile(invalidPath); err == nil {
		t.Fatal("malformed YAML was accepted")
	}
	if after := c.All(); !reflect.DeepEqual(after, before) {
		t.Fatalf("failed reload changed snapshot: before=%v after=%v", before, after)
	}
}

func TestConfigPreservesExplicitEmptyValues(t *testing.T) {
	c := New()
	c.Set("name", "")
	if got := c.GetStringDefault("name", "fallback"); got != "" {
		t.Fatalf("explicit empty string = %q, want empty", got)
	}

	t.Setenv("EMPTY_CONFIG_VALUE", "")
	got, ok := c.Get("empty.config.value")
	if !ok || got != "" {
		t.Fatalf("explicit empty environment value = (%v, %v), want (empty, true)", got, ok)
	}

	type target struct {
		Name string `env:"NAME" default:"fallback"`
	}
	t.Setenv("AUDIT_NAME", "")
	var dst target
	if err := BindEnv(&dst, "AUDIT"); err != nil {
		t.Fatalf("bind explicit empty value: %v", err)
	}
	if dst.Name != "" {
		t.Fatalf("bound explicit empty value = %q, want empty", dst.Name)
	}
}

func TestConfigZeroValueAndSnapshotsDoNotExposeMutableState(t *testing.T) {
	var c Config
	c.Set("nested", map[string]any{"token": "original"})

	value, ok := c.Get("nested")
	if !ok {
		t.Fatal("zero-value Config did not retain Set value")
	}
	value.(map[string]any)["token"] = "mutated"

	snapshot := c.All()
	snapshot["nested"].(map[string]any)["token"] = "mutated again"

	value, _ = c.Get("nested")
	if got := value.(map[string]any)["token"]; got != "original" {
		t.Fatalf("caller mutated internal config state: %v", got)
	}
}

func TestConfigKeysAreDeterministic(t *testing.T) {
	c := New()
	c.Set("z", 1)
	c.Set("a", 2)
	c.Set("m", 3)
	if got, want := c.Keys(), []string{"a", "m", "z"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Keys() = %v, want %v", got, want)
	}
}

func TestConcurrentReloadPublishesWholeSnapshots(t *testing.T) {
	pathA := writeAuditConfig(t, "a.json", `{"generation":"a","a":1}`, 0o600)
	pathB := writeAuditConfig(t, "b.json", `{"generation":"b","b":2}`, 0o600)
	c, err := Load(pathA)
	if err != nil {
		t.Fatalf("load initial config: %v", err)
	}

	start := make(chan struct{})
	errorsFound := make(chan string, 1)
	report := func(message string) {
		select {
		case errorsFound <- message:
		default:
		}
	}
	var wg sync.WaitGroup
	for writer := 0; writer < 2; writer++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			<-start
			paths := [...]string{pathA, pathB}
			for index := 0; index < 50; index++ {
				if err := c.LoadFile(paths[(index+offset)%len(paths)]); err != nil {
					report(err.Error())
					return
				}
			}
		}(writer)
	}
	for reader := 0; reader < 4; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for index := 0; index < 200; index++ {
				snapshot := c.All()
				switch snapshot["generation"] {
				case "a":
					if len(snapshot) != 2 || snapshot["a"] != json.Number("1") {
						report("observed partial generation a snapshot")
						return
					}
				case "b":
					if len(snapshot) != 2 || snapshot["b"] != json.Number("2") {
						report("observed partial generation b snapshot")
						return
					}
				default:
					report("observed snapshot without a valid generation")
					return
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	select {
	case message := <-errorsFound:
		t.Fatal(message)
	default:
	}
}

func TestJSONConfigPreservesInt64Precision(t *testing.T) {
	path := writeAuditConfig(t, "config.json", `{"id":9223372036854775807}`, 0o600)
	c, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got := c.GetInt64("id"); got != int64(9223372036854775807) {
		t.Fatalf("GetInt64() = %d, want max int64", got)
	}
	value, _ := c.Get("id")
	if number, ok := value.(json.Number); !ok || number.String() != "9223372036854775807" {
		t.Fatalf("stored number = %T(%v), want exact json.Number", value, value)
	}
}

func TestBindEnvRejectsNarrowIntegerOverflow(t *testing.T) {
	t.Setenv("AUDIT_SMALL", "128")
	var dst struct {
		Small int8 `env:"SMALL"`
	}
	if err := BindEnv(&dst, "AUDIT"); err == nil {
		t.Fatalf("overflow was silently truncated to %d", dst.Small)
	}
}

func TestLoadEnvRequiresPrefixBoundary(t *testing.T) {
	t.Setenv("APP_PORT", "8080")
	t.Setenv("APPLICATION_SECRET", "must-not-load")
	c := New()
	c.LoadEnv("APP")
	if got := c.GetInt("port"); got != 8080 {
		t.Fatalf("APP_PORT = %d, want 8080", got)
	}
	if c.Has("lication.secret") {
		t.Fatal("prefix APP matched unrelated variable APPLICATION_SECRET")
	}
}

func TestLoadEnvRejectsUnscopedImport(t *testing.T) {
	t.Setenv("TOOLKIT_AUDIT_SECRET", "must-not-load")
	c := New()
	if err := c.LoadEnv(""); !errors.Is(err, ErrInvalidPrefix) {
		t.Fatalf("LoadEnv() error = %v, want ErrInvalidPrefix", err)
	}
	if len(c.Keys()) != 0 {
		t.Fatalf("unscoped LoadEnv imported process environment: %v", c.Keys())
	}
}

func TestUnmarshalKeyRejectsUnknownFields(t *testing.T) {
	c := New()
	c.Set("database", map[string]any{"host": "localhost", "unexpected": true})
	var target struct {
		Host string `json:"host"`
	}
	if err := c.UnmarshalKey("database", &target); err == nil {
		t.Fatal("UnmarshalKey accepted an unknown field")
	}
}

func TestLoadFileRejectsFinalSymlinkAndGroupWritableFile(t *testing.T) {
	target := writeAuditConfig(t, "target.json", `{"safe":true}`, 0o600)
	link := filepath.Join(t.TempDir(), "config.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := Load(link); err == nil {
		t.Fatal("config loader followed a final symlink")
	}

	if runtime.GOOS == "windows" {
		return
	}
	insecure := writeAuditConfig(t, "insecure.json", `{"safe":true}`, 0o666)
	if _, err := Load(insecure); err == nil {
		t.Fatal("config loader accepted a group-writable file")
	}
}

func TestLoadRejectsNonObjectJSON(t *testing.T) {
	path := writeAuditConfig(t, "config.json", `null`, 0o600)
	if _, err := Load(path); err == nil {
		t.Fatal("JSON null was accepted as a configuration object")
	}
}

func TestSubsetParsersRejectUnsupportedNestedValues(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "config.yaml", content: "database:\n  host: localhost\n"},
		{name: "config.toml", content: "ports = [8080, 8081]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeAuditConfig(t, tt.name, tt.content, 0o600)
			if _, err := Load(path); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("Load() error = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

func TestSubsetParsersHandleInlineComments(t *testing.T) {
	for _, name := range []string{"config.yaml", "config.toml"} {
		t.Run(name, func(t *testing.T) {
			separator := ":"
			if filepath.Ext(name) == ".toml" {
				separator = "="
			}
			path := writeAuditConfig(t, name, "port "+separator+" 8080 # server port\n", 0o600)
			loaded, err := Load(path)
			if err != nil {
				t.Fatalf("Load() error: %v", err)
			}
			if got := loaded.GetInt("port"); got != 8080 {
				t.Fatalf("port = %d, want 8080", got)
			}
		})
	}
}

func TestSetGlobalNeverPublishesNil(t *testing.T) {
	original := Global()
	defer SetGlobal(original)
	if err := SetGlobal(nil); !errors.Is(err, ErrInvalidType) {
		t.Fatalf("SetGlobal(nil) error = %v, want ErrInvalidType", err)
	}
	if Global() == nil {
		t.Fatal("SetGlobal published a nil configuration")
	}
}

func TestLoadRejectsOversizedConfig(t *testing.T) {
	content := `{"value":"` + strings.Repeat("x", 8<<20) + `"}`
	path := writeAuditConfig(t, "config.json", content, 0o600)
	if _, err := Load(path); err == nil {
		t.Fatal("oversized config was accepted")
	}
}

func TestUnsupportedFormatIsRejectedBeforeFileAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.exe")
	if _, err := Load(path); !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("Load() error = %v, want ErrUnsupportedFormat", err)
	}
}
