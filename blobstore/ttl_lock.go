package blobstore

import (
	"errors"
	"fmt"
	"os"
)

const ttlLockFileName = ".blobstore.ttl.lock"

type ttlFileLock struct {
	file *os.File
}

func (s *Store) acquireTTLFileLock(exclusive bool) (*ttlFileLock, error) {
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, fmt.Errorf("blobstore: open root for ttl lock: %w", err)
	}
	file, openErr := root.OpenFile(ttlLockFileName, os.O_CREATE|os.O_RDWR, 0o600)
	closeRootErr := root.Close()
	if openErr != nil || closeRootErr != nil {
		var closeFileErr error
		if file != nil {
			closeFileErr = file.Close()
		}
		return nil, errors.Join(
			wrapOptionalError("blobstore: open ttl lock", openErr),
			wrapOptionalError("blobstore: close root after opening ttl lock", closeRootErr),
			wrapOptionalError("blobstore: close ttl lock after open failure", closeFileErr),
		)
	}

	info, err := file.Stat()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("blobstore: stat ttl lock: %w", err), file.Close())
	}
	if !info.Mode().IsRegular() {
		return nil, errors.Join(errors.New("blobstore: ttl lock is not a regular file"), file.Close())
	}
	if err := file.Chmod(0o600); err != nil {
		return nil, errors.Join(fmt.Errorf("blobstore: secure ttl lock permissions: %w", err), file.Close())
	}
	if err := lockBlobstoreFile(file, exclusive); err != nil {
		return nil, errors.Join(fmt.Errorf("blobstore: acquire ttl lock: %w", err), file.Close())
	}
	return &ttlFileLock{file: file}, nil
}

func (l *ttlFileLock) close() error {
	return errors.Join(
		wrapOptionalError("blobstore: release ttl lock", unlockBlobstoreFile(l.file)),
		wrapOptionalError("blobstore: close ttl lock", l.file.Close()),
	)
}

func wrapOptionalError(message string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}
