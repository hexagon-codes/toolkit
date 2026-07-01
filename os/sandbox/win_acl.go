//go:build windows

package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

// Phase 8 D30: ACL file isolation.
//
// Configures temporary DACL entries for the current AppContainer SID:
// workspace is read/write/execute, ReadablePaths are read/execute only, and
// DeniedPaths add explicit deny ACEs. Original DACLs are restored after the
// sandboxed process exits.

const (
	SE_FILE_OBJECT        = 1
	DACL_SECURITY_INFO    = 0x00000004
	GRANT_ACCESS          = 1
	SET_ACCESS            = 2
	DENY_ACCESS           = 3
	NO_INHERITANCE        = 0
	OBJECT_INHERIT_ACE    = 0x1
	CONTAINER_INHERIT_ACE = 0x2

	GENERIC_READ    = 0x80000000
	GENERIC_WRITE   = 0x40000000
	GENERIC_EXECUTE = 0x20000000

	TRUSTEE_IS_SID = 0
)

type explicitAccessW struct {
	grfAccessPermissions uint32
	grfAccessMode        uint32
	grfInheritance       uint32
	trustee              trusteeW
}

type trusteeW struct {
	pMultipleTrustee         uintptr
	multipleTrusteeOperation uint32
	trusteeForm              uint32 // TRUSTEE_IS_SID = 0
	trusteeType              uint32 // TRUSTEE_IS_WELL_KNOWN_GROUP = 5
	ptstrName                uintptr
}

var (
	modAdvapi32ACL             = syscall.NewLazyDLL("advapi32.dll")
	procSetEntriesInAclW       = modAdvapi32ACL.NewProc("SetEntriesInAclW")
	procSetNamedSecurityInfoW2 = modAdvapi32ACL.NewProc("SetNamedSecurityInfoW")
	procGetNamedSecurityInfoW  = modAdvapi32ACL.NewProc("GetNamedSecurityInfoW")
)

// aclConfig holds the original DACL for restoration.
type aclConfig struct {
	path     string
	origDACL uintptr // pointer to original ACL
	origSD   uintptr // security descriptor (for freeing)
}

type windowsACLPolicy struct {
	entries []*aclConfig
}

type windowsACLRule struct {
	path        string
	permissions uint32
	mode        uint32
}

func applyWindowsACLPolicy(cfg Config, appContainerSID []byte) (*windowsACLPolicy, error) {
	if len(appContainerSID) == 0 {
		return nil, fmt.Errorf("appcontainer SID is required")
	}
	rules, err := windowsACLRulesForConfig(cfg)
	if err != nil {
		return nil, err
	}
	policy := &windowsACLPolicy{}
	for _, rule := range rules {
		entry, err := applyPathACL(rule.path, appContainerSID, rule.permissions, rule.mode)
		if err != nil {
			_ = policy.restoreACL()
			return nil, err
		}
		policy.entries = append(policy.entries, entry)
	}
	return policy, nil
}

func windowsACLRulesForConfig(cfg Config) ([]windowsACLRule, error) {
	var rules []windowsACLRule

	workspace, ok, err := cleanWindowsACLPath(cfg.Workspace, true)
	if err != nil {
		return nil, fmt.Errorf("workspace path: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("workspace path is required")
	}
	rules = append(rules, windowsACLRule{
		path:        workspace,
		permissions: GENERIC_READ | GENERIC_WRITE | GENERIC_EXECUTE,
		mode:        GRANT_ACCESS,
	})

	for _, p := range cfg.ReadablePaths {
		clean, ok, err := cleanWindowsACLPath(p, false)
		if err != nil {
			return nil, fmt.Errorf("readable path %q: %w", p, err)
		}
		if !ok {
			continue
		}
		rules = append(rules, windowsACLRule{
			path:        clean,
			permissions: GENERIC_READ | GENERIC_EXECUTE,
			mode:        GRANT_ACCESS,
		})
	}

	for _, p := range cfg.DeniedPaths {
		clean, ok, err := cleanWindowsACLPath(p, false)
		if err != nil {
			return nil, fmt.Errorf("denied path %q: %w", p, err)
		}
		if !ok {
			continue
		}
		rules = append(rules, windowsACLRule{
			path:        clean,
			permissions: GENERIC_READ | GENERIC_WRITE | GENERIC_EXECUTE,
			mode:        DENY_ACCESS,
		})
	}

	return rules, nil
}

func applyPathACL(path string, sid []byte, permissions uint32, mode uint32) (*aclConfig, error) {
	if len(sid) == 0 {
		return nil, fmt.Errorf("SID is required")
	}
	cfg := &aclConfig{path: path}

	pathW, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("UTF16 path: %w", err)
	}
	r, _, callErr := procGetNamedSecurityInfoW.Call(
		uintptr(unsafe.Pointer(pathW)),
		SE_FILE_OBJECT,
		DACL_SECURITY_INFO,
		0, 0,
		uintptr(unsafe.Pointer(&cfg.origDACL)),
		0,
		uintptr(unsafe.Pointer(&cfg.origSD)),
	)
	if r != 0 {
		return nil, fmt.Errorf("GetNamedSecurityInfo(%s): error %d: %w", path, r, callErr)
	}

	inheritance := aclInheritanceForPath(path)
	ea := explicitAccessW{
		grfAccessPermissions: permissions,
		grfAccessMode:        mode,
		grfInheritance:       inheritance,
		trustee: trusteeW{
			trusteeForm: TRUSTEE_IS_SID,
			ptstrName:   uintptr(unsafe.Pointer(&sid[0])),
		},
	}

	var newACL uintptr
	r, _, callErr = procSetEntriesInAclW.Call(
		1, // count
		uintptr(unsafe.Pointer(&ea)),
		cfg.origDACL,
		uintptr(unsafe.Pointer(&newACL)),
	)
	if r != 0 {
		cfg.freeOriginal()
		return nil, fmt.Errorf("SetEntriesInAcl(%s): error %d: %w", path, r, callErr)
	}
	if newACL != 0 {
		defer func() {
			_, _ = syscall.LocalFree(syscall.Handle(newACL))
		}()
	}

	r, _, callErr = procSetNamedSecurityInfoW2.Call(
		uintptr(unsafe.Pointer(pathW)),
		SE_FILE_OBJECT,
		DACL_SECURITY_INFO,
		0, 0,
		newACL,
		0,
	)
	runtimeKeepAliveSID(sid)
	if r != 0 {
		cfg.freeOriginal()
		return nil, fmt.Errorf("SetNamedSecurityInfo(%s): error %d: %w", path, r, callErr)
	}

	return cfg, nil
}

func aclInheritanceForPath(path string) uint32 {
	st, err := os.Stat(path)
	if err == nil && st.IsDir() {
		return OBJECT_INHERIT_ACE | CONTAINER_INHERIT_ACE
	}
	return NO_INHERITANCE
}

// restoreACL restores the original DACL.
func (c *aclConfig) restoreACL() error {
	if c == nil {
		return nil
	}
	pathW, err := syscall.UTF16PtrFromString(c.path)
	if err != nil {
		return fmt.Errorf("UTF16 restore path: %w", err)
	}
	r, _, err := procSetNamedSecurityInfoW2.Call(
		uintptr(unsafe.Pointer(pathW)),
		SE_FILE_OBJECT,
		DACL_SECURITY_INFO,
		0, 0,
		c.origDACL,
		0,
	)
	if r != 0 {
		return fmt.Errorf("restore ACL: error %d: %w", r, err)
	}
	c.freeOriginal()
	return nil
}

func (c *aclConfig) freeOriginal() {
	if c.origSD != 0 {
		_, _ = syscall.LocalFree(syscall.Handle(c.origSD))
		c.origSD = 0
		c.origDACL = 0
	}
}

func (p *windowsACLPolicy) restoreACL() error {
	if p == nil {
		return nil
	}
	var firstErr error
	for i := len(p.entries) - 1; i >= 0; i-- {
		if err := p.entries[i].restoreACL(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	p.entries = nil
	return firstErr
}

func cleanWindowsACLPath(path string, requireExists bool) (string, bool, error) {
	path = strings.TrimSpace(expandWindowsPath(path))
	if path == "" {
		if requireExists {
			return "", false, fmt.Errorf("empty path")
		}
		return "", false, nil
	}
	if err := validateWindowsPath(path); err != nil {
		return "", false, err
	}
	if !filepath.IsAbs(path) {
		if requireExists {
			return "", false, fmt.Errorf("path must be absolute: %s", path)
		}
		return "", false, nil
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	path = filepath.Clean(path)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) && !requireExists {
			return "", false, nil
		}
		return "", false, err
	}
	return path, true, nil
}

func expandWindowsPath(path string) string {
	if path == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

func runtimeKeepAliveSID(sid []byte) {
	// Keep the backing array alive across the Win32 calls that consume ptstrName.
	runtime.KeepAlive(sid)
}

// validatePath checks for dangerous path patterns on Windows.
func validateWindowsPath(path string) error {
	// Block Alternate Data Streams (file:stream)
	if hasWindowsAlternateDataStream(path) {
		return fmt.Errorf("alternate data streams not allowed: %s", path)
	}
	// Block UNC paths
	if strings.HasPrefix(path, `\\`) {
		return fmt.Errorf("UNC paths not allowed: %s", path)
	}
	// Block device handles
	if strings.HasPrefix(strings.ToLower(path), `\\.\`) || strings.HasPrefix(strings.ToLower(path), `\\?\`) {
		return fmt.Errorf("device handles not allowed: %s", path)
	}
	return nil
}

func hasWindowsAlternateDataStream(path string) bool {
	if !strings.Contains(path, ":") {
		return false
	}
	if isAbsWindowsPath(path) {
		return strings.Contains(path[2:], ":")
	}
	return true
}

func isAbsWindowsPath(path string) bool {
	return len(path) >= 3 && path[1] == ':' && (path[2] == '\\' || path[2] == '/')
}
