//go:build windows

package sandbox

import (
	"errors"
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
	seFileObject        = 1
	daclSecurityInfo    = 0x00000004
	grantAccess         = 1
	denyAccess          = 3
	noInheritance       = 0
	objectInheritACE    = 0x1
	containerInheritACE = 0x2

	genericRead    = 0x80000000
	genericWrite   = 0x40000000
	genericExecute = 0x20000000

	trusteeIsSID = 0
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
	trusteeForm              uint32 // trusteeIsSID = 0
	trusteeType              uint32 // TRUSTEE_IS_WELL_KNOWN_GROUP = 5
	ptstrName                uintptr
}

var (
	modAdvapi32ACL             = syscall.NewLazyDLL("advapi32.dll")
	procSetEntriesInACLW       = modAdvapi32ACL.NewProc("SetEntriesInAclW")
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
		entry, applyErr := applyPathACL(rule.path, appContainerSID, rule.permissions, rule.mode)
		if applyErr != nil {
			return nil, errors.Join(applyErr, policy.restoreACL())
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
		permissions: genericRead | genericWrite | genericExecute,
		mode:        grantAccess,
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
			permissions: genericRead | genericExecute,
			mode:        grantAccess,
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
			permissions: genericRead | genericWrite | genericExecute,
			mode:        denyAccess,
		})
	}

	return rules, nil
}

func applyPathACL(path string, sid []byte, permissions, mode uint32) (*aclConfig, error) {
	if len(sid) == 0 {
		return nil, fmt.Errorf("SID is required")
	}
	cfg := &aclConfig{path: path}

	pathW, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("UTF16 path: %w", err)
	}
	r, _, callErr := procGetNamedSecurityInfoW.Call(
		uintptr(unsafe.Pointer(pathW)), // #nosec G103 -- 指针来自已校验且在调用期间存活的 UTF-16 路径。
		seFileObject,
		daclSecurityInfo,
		0, 0,
		uintptr(unsafe.Pointer(&cfg.origDACL)), // #nosec G103 -- Win32 API 同步写入原始 DACL 指针。
		0,
		uintptr(unsafe.Pointer(&cfg.origSD)), // #nosec G103 -- Win32 API 同步写入安全描述符指针。
	)
	if r != 0 {
		return nil, errors.Join(
			fmt.Errorf("GetNamedSecurityInfo(%s): error %d: %w", path, r, callErr),
			cfg.freeOriginal(),
		)
	}

	inheritance := aclInheritanceForPath(path)
	ea := explicitAccessW{
		grfAccessPermissions: permissions,
		grfAccessMode:        mode,
		grfInheritance:       inheritance,
		trustee: trusteeW{
			trusteeForm: trusteeIsSID,
			ptstrName:   uintptr(unsafe.Pointer(&sid[0])), // #nosec G103 -- SID 已校验非空并通过 KeepAlive 保持存活。
		},
	}

	var newACL uintptr
	r, _, callErr = procSetEntriesInACLW.Call(
		1,                            // 条目数量
		uintptr(unsafe.Pointer(&ea)), // #nosec G103 -- 结构体布局与 EXPLICIT_ACCESS_W ABI 一致。
		cfg.origDACL,
		uintptr(unsafe.Pointer(&newACL)), // #nosec G103 -- Win32 API 同步写入新 ACL 指针。
	)
	if r != 0 {
		return nil, errors.Join(
			fmt.Errorf("SetEntriesInAcl(%s): error %d: %w", path, r, callErr),
			cfg.freeOriginal(),
		)
	}

	r, _, callErr = procSetNamedSecurityInfoW2.Call(
		uintptr(unsafe.Pointer(pathW)), // #nosec G103 -- 指针来自已校验且在调用期间存活的 UTF-16 路径。
		seFileObject,
		daclSecurityInfo,
		0, 0,
		newACL,
		0,
	)
	runtimeKeepAliveSID(sid)
	var freeACLErr error
	if newACL != 0 {
		_, freeACLErr = syscall.LocalFree(syscall.Handle(newACL))
		if freeACLErr != nil {
			freeACLErr = fmt.Errorf("release applied ACL buffer: %w", freeACLErr)
		}
	}
	if r != 0 {
		return nil, errors.Join(
			fmt.Errorf("SetNamedSecurityInfo(%s): error %d: %w", path, r, callErr),
			freeACLErr,
			cfg.freeOriginal(),
		)
	}
	if freeACLErr != nil {
		return nil, errors.Join(freeACLErr, cfg.restoreACL())
	}

	return cfg, nil
}

func aclInheritanceForPath(path string) uint32 {
	st, err := os.Stat(path)
	if err == nil && st.IsDir() {
		return objectInheritACE | containerInheritACE
	}
	return noInheritance
}

// restoreACL restores the original DACL.
func (c *aclConfig) restoreACL() error {
	if c == nil || c.origSD == 0 {
		return nil
	}
	pathW, err := syscall.UTF16PtrFromString(c.path)
	if err != nil {
		return fmt.Errorf("UTF16 restore path: %w", err)
	}
	r, _, err := procSetNamedSecurityInfoW2.Call(
		uintptr(unsafe.Pointer(pathW)), // #nosec G103 -- 指针来自已校验且在调用期间存活的 UTF-16 路径。
		seFileObject,
		daclSecurityInfo,
		0, 0,
		c.origDACL,
		0,
	)
	if r != 0 {
		return fmt.Errorf("restore ACL: error %d: %w", r, err)
	}
	return c.freeOriginal()
}

func (c *aclConfig) freeOriginal() error {
	if c == nil || c.origSD == 0 {
		return nil
	}
	_, err := syscall.LocalFree(syscall.Handle(c.origSD))
	if err != nil {
		return fmt.Errorf("release original security descriptor: %w", err)
	}
	c.origSD = 0
	c.origDACL = 0
	return nil
}

func (p *windowsACLPolicy) restoreACL() error {
	if p == nil {
		return nil
	}
	var resultErr error
	for i := len(p.entries) - 1; i >= 0; i-- {
		resultErr = errors.Join(resultErr, p.entries[i].restoreACL())
	}
	if resultErr == nil {
		p.entries = nil
	}
	return resultErr
}

func cleanWindowsACLPath(path string, requireExists bool) (cleanPath string, exists bool, err error) {
	path = strings.TrimSpace(expandPath(path))
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
	if resolvedPath, resolveErr := filepath.EvalSymlinks(path); resolveErr == nil {
		path = resolvedPath
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
