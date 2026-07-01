//go:build windows

package sandbox

import (
	"path/filepath"
	"testing"
)

func TestWindowsACLRulesReadablePathsAreReadOnly(t *testing.T) {
	workspace := t.TempDir()
	readable := t.TempDir()
	denied := t.TempDir()

	rules, err := windowsACLRulesForConfig(Config{
		Workspace:     workspace,
		ReadablePaths: []string{readable},
		DeniedPaths:   []string{denied},
	})
	if err != nil {
		t.Fatalf("build ACL rules: %v", err)
	}
	if len(rules) != 3 {
		t.Fatalf("rules len=%d, want 3: %#v", len(rules), rules)
	}

	wsRule := mustFindACLRule(t, rules, workspace)
	if wsRule.mode != GRANT_ACCESS {
		t.Fatalf("workspace mode=%d, want GRANT_ACCESS", wsRule.mode)
	}
	if wsRule.permissions&(GENERIC_READ|GENERIC_WRITE|GENERIC_EXECUTE) != GENERIC_READ|GENERIC_WRITE|GENERIC_EXECUTE {
		t.Fatalf("workspace permissions=%#x, want RWX", wsRule.permissions)
	}

	readRule := mustFindACLRule(t, rules, readable)
	if readRule.mode != GRANT_ACCESS {
		t.Fatalf("readable mode=%d, want GRANT_ACCESS", readRule.mode)
	}
	if readRule.permissions&GENERIC_WRITE != 0 {
		t.Fatalf("readable permissions=%#x unexpectedly include write", readRule.permissions)
	}
	if readRule.permissions&(GENERIC_READ|GENERIC_EXECUTE) != GENERIC_READ|GENERIC_EXECUTE {
		t.Fatalf("readable permissions=%#x, want read+execute", readRule.permissions)
	}

	denyRule := mustFindACLRule(t, rules, denied)
	if denyRule.mode != DENY_ACCESS {
		t.Fatalf("denied mode=%d, want DENY_ACCESS", denyRule.mode)
	}
	if denyRule.permissions&(GENERIC_READ|GENERIC_WRITE|GENERIC_EXECUTE) != GENERIC_READ|GENERIC_WRITE|GENERIC_EXECUTE {
		t.Fatalf("denied permissions=%#x, want RWX deny", denyRule.permissions)
	}
}

func TestWindowsACLRulesSkipMissingOptionalPaths(t *testing.T) {
	workspace := t.TempDir()
	missing := filepath.Join(workspace, "missing")

	rules, err := windowsACLRulesForConfig(Config{
		Workspace:     workspace,
		ReadablePaths: []string{missing},
		DeniedPaths:   []string{missing},
	})
	if err != nil {
		t.Fatalf("build ACL rules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("rules len=%d, want only workspace rule: %#v", len(rules), rules)
	}
}

func TestWindowsValidatePathRejectsAbsoluteADS(t *testing.T) {
	if err := validateWindowsPath(`C:\Users\test\file.txt:hidden`); err == nil {
		t.Fatal("expected absolute ADS path to be rejected")
	}
}

func mustFindACLRule(t *testing.T, rules []windowsACLRule, path string) windowsACLRule {
	t.Helper()
	want, err := filepath.EvalSymlinks(path)
	if err != nil {
		want = filepath.Clean(path)
	}
	want = filepath.Clean(want)
	for _, rule := range rules {
		if filepath.Clean(rule.path) == want {
			return rule
		}
	}
	t.Fatalf("ACL rule for %q not found in %#v", want, rules)
	return windowsACLRule{}
}
