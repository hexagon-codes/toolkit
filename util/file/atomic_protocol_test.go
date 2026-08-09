package file

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAtomicReplaceOrdersDurabilitySteps(t *testing.T) {
	path := filepath.Join("parent", "state.json")
	events := make([]string, 0, 8)
	ops := recordingAtomicWriteOps(&events, nil)

	err := atomicReplaceWithOps(path, 0o640, func(w io.Writer) error {
		_, writeErr := w.Write([]byte("data"))
		return writeErr
	}, ops)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"create:parent:.state.json.tmp-*",
		"write",
		"chmod:0640",
		"sync-file",
		"close-file",
		"rename:parent/.state.json.tmp-fixed->parent/state.json",
		"sync-dir:parent",
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
}

func TestAtomicReplaceFailureCleanupBoundary(t *testing.T) {
	tests := []struct {
		name       string
		failure    string
		wantRename bool
		wantRemove bool
	}{
		{name: "write", failure: "write", wantRemove: true},
		{name: "chmod", failure: "chmod", wantRemove: true},
		{name: "file sync", failure: "sync-file", wantRemove: true},
		{name: "file close", failure: "close-file", wantRemove: true},
		{name: "rename", failure: "rename", wantRename: true, wantRemove: true},
		{name: "directory sync", failure: "sync-dir", wantRename: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join("parent", "state.json")
			events := make([]string, 0, 9)
			injected := errors.New("injected " + test.failure + " failure")
			failures := map[string]error{test.failure: injected}
			ops := recordingAtomicWriteOps(&events, failures)

			err := atomicReplaceWithOps(path, 0o600, func(w io.Writer) error {
				_, writeErr := w.Write([]byte("data"))
				return writeErr
			}, ops)
			if !errors.Is(err, injected) {
				t.Fatalf("error = %v, want injected failure", err)
			}
			if got := hasEventPrefix(events, "rename:"); got != test.wantRename {
				t.Fatalf("rename event = %v, want %v; events = %#v", got, test.wantRename, events)
			}
			if got := hasEventPrefix(events, "remove:"); got != test.wantRemove {
				t.Fatalf("remove event = %v, want %v; events = %#v", got, test.wantRemove, events)
			}
			if got := countEvent(events, "close-file"); got != 1 {
				t.Fatalf("close count = %d, want 1; events = %#v", got, events)
			}
		})
	}
}

func TestAtomicReplaceJoinsCleanupFailure(t *testing.T) {
	path := filepath.Join("parent", "state.json")
	events := make([]string, 0, 9)
	renameErr := errors.New("injected rename failure")
	removeErr := errors.New("injected remove failure")
	ops := recordingAtomicWriteOps(&events, map[string]error{
		"rename": renameErr,
		"remove": removeErr,
	})

	err := atomicReplaceWithOps(path, 0o600, func(w io.Writer) error {
		_, writeErr := w.Write([]byte("data"))
		return writeErr
	}, ops)
	if !errors.Is(err, renameErr) || !errors.Is(err, removeErr) {
		t.Fatalf("error = %v, want both rename and cleanup failures", err)
	}
}

func TestAtomicReplaceRejectsNilPopulate(t *testing.T) {
	events := make([]string, 0, 1)
	err := atomicReplaceWithOps("state.json", 0o600, nil, recordingAtomicWriteOps(&events, nil))
	if err == nil {
		t.Fatal("atomicReplaceWithOps accepted a nil populate function")
	}
	if len(events) != 0 {
		t.Fatalf("filesystem operations ran for invalid input: %#v", events)
	}
}

type recordingAtomicFile struct {
	events   *[]string
	name     string
	failures map[string]error
}

func (f *recordingAtomicFile) Write(p []byte) (int, error) {
	*f.events = append(*f.events, "write")
	if err := f.failures["write"]; err != nil {
		return 0, err
	}
	return len(p), nil
}

func (f *recordingAtomicFile) Chmod(perm os.FileMode) error {
	*f.events = append(*f.events, fmt.Sprintf("chmod:%04o", perm.Perm()))
	return f.failures["chmod"]
}

func (f *recordingAtomicFile) Sync() error {
	*f.events = append(*f.events, "sync-file")
	return f.failures["sync-file"]
}

func (f *recordingAtomicFile) Close() error {
	*f.events = append(*f.events, "close-file")
	return f.failures["close-file"]
}

func (f *recordingAtomicFile) Name() string {
	return f.name
}

func recordingAtomicWriteOps(events *[]string, failures map[string]error) atomicWriteOps {
	if failures == nil {
		failures = make(map[string]error)
	}
	tempName := filepath.Join("parent", ".state.json.tmp-fixed")
	return atomicWriteOps{
		createTemp: func(dir, pattern string) (atomicWriteFile, error) {
			*events = append(*events, "create:"+dir+":"+pattern)
			if err := failures["create"]; err != nil {
				return nil, err
			}
			return &recordingAtomicFile{events: events, name: tempName, failures: failures}, nil
		},
		rename: func(oldPath, newPath string) error {
			*events = append(*events, "rename:"+oldPath+"->"+newPath)
			return failures["rename"]
		},
		remove: func(path string) error {
			*events = append(*events, "remove:"+path)
			return failures["remove"]
		},
		syncDir: func(path string) error {
			*events = append(*events, "sync-dir:"+path)
			return failures["sync-dir"]
		},
	}
}

func hasEventPrefix(events []string, prefix string) bool {
	for _, event := range events {
		if len(event) >= len(prefix) && event[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func countEvent(events []string, target string) int {
	count := 0
	for _, event := range events {
		if event == target {
			count++
		}
	}
	return count
}
