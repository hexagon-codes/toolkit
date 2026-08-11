//go:build !windows

package sandbox

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type failingPOSIXShebangFile struct {
	*os.File
	readErr  error
	closeErr error
}

func (f *failingPOSIXShebangFile) Read([]byte) (int, error) { return 0, f.readErr }
func (f *failingPOSIXShebangFile) Close() error {
	return errors.Join(f.File.Close(), f.closeErr)
}

func TestInspectPOSIXShebangFailsClosedOnOpenReadAndCloseErrors(t *testing.T) {
	tests := []struct {
		name string
		open func(string) (posixShebangFile, error)
		want string
	}{
		{
			name: "open",
			open: func(string) (posixShebangFile, error) {
				return nil, errors.New("open sentinel")
			},
			want: "open sentinel",
		},
		{
			name: "read",
			open: func(path string) (posixShebangFile, error) {
				file, err := os.Open(path)
				if err != nil {
					return nil, err
				}
				return &failingPOSIXShebangFile{File: file, readErr: errors.New("read sentinel")}, nil
			},
			want: "read sentinel",
		},
		{
			name: "close",
			open: func(path string) (posixShebangFile, error) {
				file, err := os.Open(path)
				if err != nil {
					return nil, err
				}
				return &failingPOSIXShebangFile{File: file, closeErr: errors.New("close sentinel")}, nil
			},
			want: "close sentinel",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			script := filepath.Join(t.TempDir(), "payload")
			if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			_, err := inspectPOSIXShebangCommandsWithOpen(script, test.open)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("inspect error = %v, want %q", err, test.want)
			}
		})
	}
}

type swappingPOSIXShebangFile struct {
	*os.File
	path        string
	replacement string
	swapped     bool
}

func (f *swappingPOSIXShebangFile) Read(buffer []byte) (int, error) {
	n, err := f.File.Read(buffer)
	if !f.swapped {
		f.swapped = true
		if renameErr := os.Rename(f.replacement, f.path); renameErr != nil {
			return n, errors.Join(err, renameErr)
		}
	}
	return n, err
}

func TestInspectPOSIXShebangFailsClosedWhenPathIdentityChanges(t *testing.T) {
	directory := t.TempDir()
	script := filepath.Join(directory, "payload")
	replacement := filepath.Join(directory, "replacement")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(replacement, []byte("#!/usr/bin/env python3\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := inspectPOSIXShebangCommandsWithOpen(script, func(path string) (posixShebangFile, error) {
		file, openErr := os.Open(path)
		if openErr != nil {
			return nil, openErr
		}
		return &swappingPOSIXShebangFile{File: file, path: path, replacement: replacement}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "changed during inspection") {
		t.Fatalf("inspect error = %v, want identity change rejection", err)
	}
}

func TestInspectPOSIXShebangAcceptsEOFAndReturnsInterpreterChain(t *testing.T) {
	script := filepath.Join(t.TempDir(), "payload")
	if err := os.WriteFile(script, []byte("#!/usr/bin/env -S python3 -I"), 0o700); err != nil {
		t.Fatal(err)
	}
	commands, err := inspectPOSIXShebangCommandsWithOpen(script, func(path string) (posixShebangFile, error) {
		return os.Open(path)
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(commands, ","); got != "/usr/bin/env,python3" {
		t.Fatalf("commands = %q", got)
	}
}
