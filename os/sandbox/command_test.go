package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSandboxExecUsesOnlyStructuredCommandContract(t *testing.T) {
	sandboxType := reflect.TypeOf((*Sandbox)(nil)).Elem()
	method, ok := sandboxType.MethodByName("Exec")
	if !ok {
		t.Fatal("Sandbox.Exec is missing")
	}
	wantCommand := reflect.TypeOf(Command{})
	found := false
	for index := 0; index < method.Type.NumIn(); index++ {
		if method.Type.In(index) == wantCommand {
			found = true
		}
		if method.Type.In(index).Kind() == reflect.String {
			t.Fatal("Sandbox.Exec still exposes the legacy string command parameter")
		}
	}
	if !found {
		t.Fatalf("Sandbox.Exec type = %s, want Command parameter", method.Type)
	}
	if method.Type.NumIn() != 2 || method.Type.In(0) != reflect.TypeOf((*context.Context)(nil)).Elem() {
		t.Fatalf("Sandbox.Exec type = %s, want func(context.Context, Command)", method.Type)
	}
}

func TestPrepareSandboxCommandValidatesDirectoryAndCompleteEnvironment(t *testing.T) {
	workspace := t.TempDir()
	nested := filepath.Join(workspace, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	prepared, err := prepareSandboxCommand(Config{Workspace: workspace}, Command{
		Path: "/usr/bin/env",
		Dir:  "nested",
		Env:  []string{"LANG=C", "EMPTY="},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	absoluteNested, err := filepath.Abs(nested)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Dir != filepath.Clean(absoluteNested) {
		t.Fatalf("prepared directory = %q, want raw absolute identity %q", prepared.Dir, filepath.Clean(absoluteNested))
	}
	if !reflect.DeepEqual(prepared.Env, []string{"LANG=C", "EMPTY="}) {
		t.Fatalf("prepared environment = %q", prepared.Env)
	}

	outside := t.TempDir()
	_, err = prepareSandboxCommand(Config{Workspace: workspace}, Command{Path: "/usr/bin/env", Dir: outside}, func() ([]string, error) {
		return []string{"LANG=C"}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "must be within the workspace") {
		t.Fatalf("outside directory error = %v", err)
	}
}

func TestPrepareSandboxCommandRejectsDangerousOrMalformedEnvironment(t *testing.T) {
	workspace := t.TempDir()
	for name, env := range map[string][]string{
		"missing-equals": {"LANG"},
		"invalid-name":   {"BAD-NAME=value"},
		"duplicate":      {"LANG=C", "LANG=en_US.UTF-8"},
		"nul":            {"LANG=C\x00ignored"},
		"ld":             {"LD_PRELOAD=/tmp/inject.so"},
		"dyld":           {"DYLD_INSERT_LIBRARIES=/tmp/inject.dylib"},
		"gconv":          {"GCONV_PATH=/tmp/modules"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := prepareSandboxCommand(Config{Workspace: workspace}, Command{
				Path: "/usr/bin/env",
				Env:  env,
			}, nil)
			if err == nil {
				t.Fatalf("environment %q was accepted", env)
			}
		})
	}
}

func TestPrepareSandboxCommandUsesDefaultEnvironmentOnlyForNil(t *testing.T) {
	workspace := t.TempDir()
	defaultCalls := 0
	defaultEnvironment := func() ([]string, error) {
		defaultCalls++
		return []string{"LANG=C"}, nil
	}
	withDefault, err := prepareSandboxCommand(Config{Workspace: workspace}, Command{Path: "/usr/bin/env"}, defaultEnvironment)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(withDefault.Env, []string{"LANG=C"}) || defaultCalls != 1 {
		t.Fatalf("nil environment = %q, default calls=%d", withDefault.Env, defaultCalls)
	}
	explicitEmpty, err := prepareSandboxCommand(Config{Workspace: workspace}, Command{Path: "/usr/bin/env", Env: []string{}}, defaultEnvironment)
	if err != nil {
		t.Fatal(err)
	}
	if explicitEmpty.Env == nil || len(explicitEmpty.Env) != 0 || defaultCalls != 1 {
		t.Fatalf("explicit empty environment = %#v, default calls=%d", explicitEmpty.Env, defaultCalls)
	}
}
