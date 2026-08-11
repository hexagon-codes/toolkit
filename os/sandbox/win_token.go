//go:build windows

package sandbox

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Phase 8 D29: Restricted Token management
//
// Creates tokens with minimal privileges for sandboxed process execution.
// Aligns with Codex codex-windows-sandbox Restricted Token approach.

const (
	disableMaxPrivilege    = 0x1
	tokenIntegrityLevel    = 25
	securityGroupIntegrity = 0x00000020
)

// restrictedTokenFlags 返回创建受限令牌时唯一允许启用的标志。
// 禁止启用 SANDBOX_INERT，避免绕过 AppLocker 与软件限制策略。
func restrictedTokenFlags() uint32 {
	return disableMaxPrivilege
}

// createSandboxToken 创建受限令牌：移除全部权限并设置低完整性级别。
func createSandboxToken() (syscall.Token, error) {
	// 获取当前进程令牌。
	var processToken syscall.Token
	process, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0, fmt.Errorf("get current process: %w", err)
	}
	if err := syscall.OpenProcessToken(process, syscall.TOKEN_ALL_ACCESS, &processToken); err != nil {
		return 0, fmt.Errorf("open process token: %w", err)
	}
	// 创建受限令牌并移除全部权限。
	var restrictedToken syscall.Token
	r, _, callErr := procCreateRestrictedToken.Call(
		uintptr(processToken),
		uintptr(restrictedTokenFlags()),
		0, 0, // 不额外禁用 SID
		0, 0, // 权限由 disableMaxPrivilege 统一移除
		0, 0, // 不额外限制 SID
		uintptr(unsafe.Pointer(&restrictedToken)), // #nosec G103 -- Win32 API 同步写入有效令牌变量。
	)
	if r == 0 {
		return 0, errors.Join(fmt.Errorf("CreateRestrictedToken: %w", callErr), processToken.Close())
	}
	if closeErr := processToken.Close(); closeErr != nil {
		return 0, errors.Join(fmt.Errorf("close process token: %w", closeErr), restrictedToken.Close())
	}

	// AppContainer 使用标准低完整性级别；工作区持久低完整性标签允许其受控写入。
	if err := setTokenIntegrityLevel(restrictedToken, securityMandatoryLowRID); err != nil {
		return 0, errors.Join(fmt.Errorf("set integrity level: %w", err), restrictedToken.Close())
	}

	return restrictedToken, nil
}

// setTokenIntegrityLevel 设置令牌的强制完整性级别。
func setTokenIntegrityLevel(token syscall.Token, level uint32) error {
	sid, err := allocateSID(windows.SECURITY_MANDATORY_LABEL_AUTHORITY, level)
	if err != nil {
		return fmt.Errorf("allocate integrity SID: %w", err)
	}

	info := windows.Tokenmandatorylabel{
		Label: windows.SIDAndAttributes{Sid: sid, Attributes: securityGroupIntegrity},
	}

	r2, _, callErr := procSetTokenInformation.Call(
		uintptr(token),
		uintptr(tokenIntegrityLevel),
		uintptr(unsafe.Pointer(&info)), // #nosec G103 -- 结构体布局与 TOKEN_MANDATORY_LABEL ABI 一致。
		uintptr(info.Size()),
	)
	runtime.KeepAlive(info)
	if r2 == 0 {
		return fmt.Errorf("SetTokenInformation: %w", callErr)
	}
	return nil
}
