//go:build darwin || linux

package blobstore

import (
	"os"

	"golang.org/x/sys/unix"
)

func lockBlobstoreFile(file *os.File, exclusive bool) error {
	operation := unix.LOCK_SH
	if exclusive {
		operation = unix.LOCK_EX
	}
	for {
		err := unix.Flock(int(file.Fd()), operation)
		if err != unix.EINTR {
			return err
		}
	}
}

func unlockBlobstoreFile(file *os.File) error {
	for {
		err := unix.Flock(int(file.Fd()), unix.LOCK_UN)
		if err != unix.EINTR {
			return err
		}
	}
}
