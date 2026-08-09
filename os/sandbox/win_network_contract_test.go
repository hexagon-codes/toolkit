//go:build windows

package sandbox

import (
	"bytes"
	"testing"

	"golang.org/x/sys/windows"
)

type windowsSIDAllocatorContract func(...uint32) (*windows.SID, error)
type windowsSIDCopierContract func(*windows.SID) ([]byte, error)

var (
	_ windowsSIDAllocatorContract = allocateAppPackageSID
	_ windowsSIDCopierContract    = copySIDBytes
)

func TestWindowsSIDHelpersUseTypedPointers(t *testing.T) {
	sid, err := allocateAppPackageSID(appCapabilitySID, internetClientSID)
	if err != nil {
		t.Fatalf("allocate capability SID: %v", err)
	}

	got, err := copySIDBytes(sid)
	if err != nil {
		t.Fatalf("copy capability SID: %v", err)
	}
	want := []byte{
		1, 2, // SID 修订号和子授权项数量
		0, 0, 0, 0, 0, appPackageAuthority,
		appCapabilitySID, 0, 0, 0,
		internetClientSID, 0, 0, 0,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("capability SID mismatch: got %v, want %v", got, want)
	}
}

func TestWindowsCopySIDBytesRejectsNil(t *testing.T) {
	if _, err := copySIDBytes(nil); err == nil {
		t.Fatal("copySIDBytes(nil) returned nil error")
	}
}
