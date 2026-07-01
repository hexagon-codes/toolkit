//go:build windows

package sandbox

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"syscall"
	"unsafe"
)

// Phase 8 D31: Low Box Token network isolation

var (
	modNtdll                        = syscall.NewLazyDLL("ntdll.dll")
	procNtCreateLowBoxToken         = modNtdll.NewProc("NtCreateLowBoxToken")
	procRtlAllocateAndInitializeSid = modNtdll.NewProc("RtlAllocateAndInitializeSid")
	procRtlFreeSid                  = modNtdll.NewProc("RtlFreeSid")
)

// sidIdentifierAuthority is the SID authority structure.
type sidIdentifierAuthority struct {
	Value [6]byte
}

type sidAndAttributes struct {
	Sid        uintptr
	Attributes uint32
}

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
	appContainerSid, err := allocateUniqueAppContainerSID()
	if err != nil {
		return 0, nil, err
	}
	defer procRtlFreeSid.Call(appContainerSid)

	appContainerSIDBytes, err := copySIDBytes(appContainerSid)
	if err != nil {
		return 0, nil, fmt.Errorf("copy appcontainer SID: %w", err)
	}

	var capCount uintptr
	var capPtr uintptr
	var caps []sidAndAttributes
	if allowNetwork {
		internetSid, err := allocateAppPackageSID(appCapabilitySID, internetClientSID)
		if err != nil {
			return 0, nil, fmt.Errorf("allocate internetClient capability SID: %w", err)
		}
		defer procRtlFreeSid.Call(internetSid)
		caps = append(caps, sidAndAttributes{Sid: internetSid, Attributes: seGroupEnabled})
		capCount = uintptr(len(caps))
		capPtr = uintptr(unsafe.Pointer(&caps[0]))
	}

	var lowBoxToken syscall.Token
	r, _, err := procNtCreateLowBoxToken.Call(
		uintptr(unsafe.Pointer(&lowBoxToken)),
		uintptr(baseToken),
		syscall.TOKEN_ALL_ACCESS,
		0, // object attributes
		appContainerSid,
		capCount,
		capPtr,
		0, 0, // no handles
	)
	if r != 0 {
		return 0, nil, fmt.Errorf("NtCreateLowBoxToken: NTSTATUS 0x%X: %w", r, err)
	}

	return lowBoxToken, appContainerSIDBytes, nil
}

func allocateUniqueAppContainerSID() (uintptr, error) {
	var seed [appContainerRandomRIDSize]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return 0, fmt.Errorf("generate appcontainer SID entropy: %w", err)
	}
	subAuthorities := make([]uintptr, 0, appContainerSubAuthCount)
	subAuthorities = append(subAuthorities, appContainerBaseSID)
	for i := 0; i < appContainerSubAuthCount-1; i++ {
		rid := binary.LittleEndian.Uint32(seed[i*4 : (i+1)*4])
		if rid == 0 {
			rid = uint32(i + 1)
		}
		subAuthorities = append(subAuthorities, uintptr(rid))
	}
	return allocateAppPackageSID(subAuthorities...)
}

func allocateAppPackageSID(subAuthorities ...uintptr) (uintptr, error) {
	if len(subAuthorities) == 0 || len(subAuthorities) > 8 {
		return 0, fmt.Errorf("invalid SID sub-authority count: %d", len(subAuthorities))
	}
	authority := sidIdentifierAuthority{Value: [6]byte{0, 0, 0, 0, 0, appPackageAuthority}}
	var rid [8]uintptr
	copy(rid[:], subAuthorities)
	var sid uintptr
	r, _, err := procRtlAllocateAndInitializeSid.Call(
		uintptr(unsafe.Pointer(&authority)),
		uintptr(len(subAuthorities)),
		rid[0], rid[1], rid[2], rid[3], rid[4], rid[5], rid[6], rid[7],
		uintptr(unsafe.Pointer(&sid)),
	)
	if r != 0 {
		return 0, fmt.Errorf("RtlAllocateAndInitializeSid: NTSTATUS 0x%X: %w", r, err)
	}
	return sid, nil
}

func copySIDBytes(sid uintptr) ([]byte, error) {
	if sid == 0 {
		return nil, fmt.Errorf("nil SID")
	}
	sidPtr := (*syscall.SID)(unsafe.Pointer(sid))
	size := syscall.GetLengthSid(sidPtr)
	if size == 0 {
		return nil, fmt.Errorf("empty SID")
	}
	out := make([]byte, size)
	if err := syscall.CopySid(size, (*syscall.SID)(unsafe.Pointer(&out[0])), sidPtr); err != nil {
		return nil, err
	}
	return out, nil
}
