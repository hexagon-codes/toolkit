//go:build !windows

package sandbox

import (
	"os"
	"time"
)

// sandboxCreationTime 非 Windows 平台不提供创建时间身份。
func sandboxCreationTime(string) time.Time {
	return time.Time{}
}

// sandboxIdentityExtraMatches 非 Windows 平台无额外身份判据。
func sandboxIdentityExtraMatches(sandboxPathIdentity, os.FileInfo) bool {
	return true
}
