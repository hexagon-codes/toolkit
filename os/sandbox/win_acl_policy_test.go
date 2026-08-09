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
	if wsRule.mode != grantAccess {
		t.Fatalf("workspace mode=%d, want grantAccess", wsRule.mode)
	}
	if wsRule.permissions&(genericRead|genericWrite|genericExecute) != genericRead|genericWrite|genericExecute {
		t.Fatalf("workspace permissions=%#x, want RWX", wsRule.permissions)
	}

	readRule := mustFindACLRule(t, rules, readable)
	if readRule.mode != grantAccess {
		t.Fatalf("readable mode=%d, want grantAccess", readRule.mode)
	}
	if readRule.permissions&genericWrite != 0 {
		t.Fatalf("readable permissions=%#x unexpectedly include write", readRule.permissions)
	}
	if readRule.permissions&(genericRead|genericExecute) != genericRead|genericExecute {
		t.Fatalf("readable permissions=%#x, want read+execute", readRule.permissions)
	}

	denyRule := mustFindACLRule(t, rules, denied)
	if denyRule.mode != denyAccess {
		t.Fatalf("denied mode=%d, want denyAccess", denyRule.mode)
	}
	if denyRule.permissions&(genericRead|genericWrite|genericExecute) != genericRead|genericWrite|genericExecute {
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
