//go:build !windows

package sandbox

import (
	"os"
)

// sandboxCreationTime 非 Windows 平台不提供创建时间身份。
func sandboxCreationTime(string) uint64 {
	return 0
}

// sandboxIdentityExtraMatches 非 Windows 平台无额外身份判据。
func sandboxIdentityExtraMatches(sandboxPathIdentity, os.FileInfo) bool {
	return true
}

// sandboxPathIsReparsePoint 非 Windows 平台无 junction 属性位问题。
func sandboxPathIsReparsePoint(string) bool {
	return false
}

// sandboxFileID 非 Windows 平台无文件 ID 身份。
func sandboxFileID(string) [16]byte {
	return [16]byte{}
}
