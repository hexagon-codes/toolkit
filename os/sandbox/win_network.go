//go:build windows

package sandbox

import (
	"crypto/rand"
	"encoding/binary"
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
	// Well-known AppContainer capability S-1-15-3-1, equivalent to internetClient.
	appPackageAuthority       = 15
	appContainerBaseSID       = 2
	appCapabilitySID          = 3
	internetClientSID         = 1
	appContainerSubAuthCount  = 8
	appContainerRandomRIDSize = (appContainerSubAuthCount - 1) * 4
	seGroupEnabled            = 0x00000004
)

// createLowBoxToken creates a Low Box Token for network isolation.
//
// If allowNetwork is false, the token has no capabilities (kernel-level network block).
// If allowNetwork is true, basic network capability is granted.
func createLowBoxToken(baseToken syscall.Token, allowNetwork bool) (syscall.Token, []byte, error) {
	appContainerSID, err := allocateUniqueAppContainerSID()
	if err != nil {
		return 0, nil, err
	}
	appContainerSIDBytes, err := copySIDBytes(appContainerSID)
	if err != nil {
		return 0, nil, fmt.Errorf("copy appcontainer SID: %w", err)
	}

	var capCount uintptr
	var capPtr *windows.SIDAndAttributes
	var caps []windows.SIDAndAttributes
	if allowNetwork {
		internetSID, allocateErr := allocateAppPackageSID(appCapabilitySID, internetClientSID)
		if allocateErr != nil {
			return 0, nil, fmt.Errorf("allocate internetClient capability SID: %w", allocateErr)
		}
		caps = append(caps, windows.SIDAndAttributes{Sid: internetSID, Attributes: seGroupEnabled})
		capCount = uintptr(len(caps))
		capPtr = &caps[0]
	}

	var lowBoxToken syscall.Token
	r, _, err := procNtCreateLowBoxToken.Call(
		uintptr(unsafe.Pointer(&lowBoxToken)), // #nosec G103 -- NtCreateLowBoxToken 同步写入令牌变量。
		uintptr(baseToken),
		syscall.TOKEN_ALL_ACCESS,
		0,                                        // object attributes
		uintptr(unsafe.Pointer(appContainerSID)), // #nosec G103 -- 强类型 SID 指针仅在同步系统调用边界转换。
		capCount,
		uintptr(unsafe.Pointer(capPtr)), // #nosec G103 -- 强类型能力数组指针仅在同步系统调用边界转换。
		0, 0,                            // no handles
	)
	runtime.KeepAlive(appContainerSID)
	runtime.KeepAlive(caps)
	if r != 0 {
		return 0, nil, fmt.Errorf("NtCreateLowBoxToken: NTSTATUS 0x%X: %w", r, err)
	}

	return lowBoxToken, appContainerSIDBytes, nil
}

func allocateUniqueAppContainerSID() (*windows.SID, error) {
	var seed [appContainerRandomRIDSize]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return nil, fmt.Errorf("generate appcontainer SID entropy: %w", err)
	}
	subAuthorities := make([]uint32, 0, appContainerSubAuthCount)
	subAuthorities = append(subAuthorities, appContainerBaseSID)
	for i := 0; i < appContainerSubAuthCount-1; i++ {
		rid := binary.LittleEndian.Uint32(seed[i*4 : (i+1)*4])
		if rid == 0 {
			rid = uint32(i + 1)
		}
		subAuthorities = append(subAuthorities, rid)
	}
	return allocateAppPackageSID(subAuthorities...)
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
