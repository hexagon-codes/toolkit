//go:build !darwin && !linux && !windows

package file

import (
	"errors"
	"os"
)

func openAppendFileNoFollow(string, string, os.FileMode) (*os.File, error) {
	// 未验证的平台必须显式拒绝，不能静默退化为跟随符号链接。
	return nil, errors.New("secure append is unsupported on this platform")
}
