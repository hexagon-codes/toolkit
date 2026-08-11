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

// Phase 8 D31: Low Box Token network isolation

var (
	modNtdll                = syscall.NewLazyDLL("ntdll.dll")
	procNtCreateLowBoxToken = modNtdll.NewProc("NtCreateLowBoxToken")
)

const (
	appPackageAuthority      = 15
	appContainerBaseSID      = 2
	appContainerSubAuthCount = 8
)

// createLowBoxToken 使用工作区预先派生的稳定 AppContainer SID 创建 LowBox 令牌。
// Windows 仅接受 NetworkDisabled，不授予任何网络能力。
func createLowBoxToken(
	baseToken syscall.Token,
	appContainerSIDBytes []byte,
	networkMode NetworkMode,
) (syscall.Token, error) {
	if networkMode != NetworkDisabled {
		return 0, fmt.Errorf("%w: Windows cannot provide the complete host network view", ErrUnsupportedNetworkPolicy)
	}
	appContainerSID, err := windowsSIDFromBytes(appContainerSIDBytes)
	if err != nil {
		return 0, err
	}

	var lowBoxToken syscall.Token
	r, _, err := procNtCreateLowBoxToken.Call(
		uintptr(unsafe.Pointer(&lowBoxToken)), // #nosec G103 -- NtCreateLowBoxToken 同步写入令牌变量。
		uintptr(baseToken),
		syscall.TOKEN_ALL_ACCESS,
		0,                                        // object attributes
		uintptr(unsafe.Pointer(appContainerSID)), // #nosec G103 -- 强类型 SID 指针仅在同步系统调用边界转换。
		0,
		0,    // 不授予网络或其他 AppContainer capability。
		0, 0, // no handles
	)
	runtime.KeepAlive(appContainerSID)
	runtime.KeepAlive(appContainerSIDBytes)
	if r != 0 {
		return 0, fmt.Errorf("NtCreateLowBoxToken: NTSTATUS 0x%X: %w", r, err)
	}

	return lowBoxToken, nil
}

func allocateAppPackageSID(subAuthorities ...uint32) (*windows.SID, error) {
	authority := windows.SidIdentifierAuthority{Value: [6]byte{0, 0, 0, 0, 0, appPackageAuthority}}
	return allocateSID(authority, subAuthorities...)
}

func allocateSID(authority windows.SidIdentifierAuthority, subAuthorities ...uint32) (*windows.SID, error) {
	if len(subAuthorities) == 0 || len(subAuthorities) > 8 {
		return nil, fmt.Errorf("invalid SID sub-authority count: %d", len(subAuthorities))
	}
	var rid [8]uint32
	copy(rid[:], subAuthorities)
	var sid *windows.SID
	if err := windows.AllocateAndInitializeSid(
		&authority,
		byte(len(subAuthorities)),
		rid[0], rid[1], rid[2], rid[3], rid[4], rid[5], rid[6], rid[7],
		&sid,
	); err != nil {
		return nil, fmt.Errorf("AllocateAndInitializeSid: %w", err)
	}
	copySID, copyErr := sid.Copy()
	freeErr := windows.FreeSid(sid)
	if copyErr != nil {
		if freeErr != nil {
			return nil, errors.Join(
				fmt.Errorf("copy allocated SID: %w", copyErr),
				fmt.Errorf("free allocated SID: %w", freeErr),
			)
		}
		return nil, fmt.Errorf("copy allocated SID: %w", copyErr)
	}
	if freeErr != nil {
		return nil, fmt.Errorf("free allocated SID: %w", freeErr)
	}
	return copySID, nil
}

func copySIDBytes(sid *windows.SID) ([]byte, error) {
	if sid == nil {
		return nil, fmt.Errorf("nil SID")
	}
	if !sid.IsValid() {
		return nil, fmt.Errorf("invalid SID")
	}
	size := windows.GetLengthSid(sid)
	if size == 0 {
		return nil, fmt.Errorf("empty SID")
	}
	source := unsafe.Slice((*byte)(unsafe.Pointer(sid)), int(size)) // #nosec G103 -- 已验证 SID 的长度限定原生只读视图边界。
	out := append([]byte(nil), source...)
	runtime.KeepAlive(sid)
	return out, nil
}
