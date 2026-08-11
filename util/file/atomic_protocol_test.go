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
	dir := filepath.Dir(path)
	events := make([]string, 0, 12)
	ops := recordingAtomicWriteOps(&events, nil)

	err := atomicReplaceWithOps(path, 0o640, func(w io.Writer) error {
		_, writeErr := w.Write([]byte("data"))
		return writeErr
	}, ops)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"open-parent:" + dir,
		"create:.state.json.tmp-",
		"write",
		"chmod:0640",
		"sync-file",
		"close-file",
		"verify-parent-before",
		"lstat:state.json",
		"rename:.state.json.tmp-fixed->state.json",
		"sync-dir",
		"verify-parent-after",
		"close-parent",
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
		{name: "parent identity", failure: "verify-parent-before", wantRemove: true},
		{name: "destination lstat", failure: "lstat", wantRemove: true},
		{name: "rename", failure: "rename", wantRename: true, wantRemove: true},
		{name: "directory sync", failure: "sync-dir", wantRename: true},
		{name: "parent identity after publish", failure: "verify-parent-after", wantRename: true},
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

type recordingAtomicDirectory struct {
	events      *[]string
	failures    map[string]error
	verifyCalls int
}

func (d *recordingAtomicDirectory) CreateTemp(prefix string) (atomicWriteFile, string, error) {
	*d.events = append(*d.events, "create:"+prefix)
	if err := d.failures["create"]; err != nil {
		return nil, "", err
	}
	return &recordingAtomicFile{events: d.events, failures: d.failures}, ".state.json.tmp-fixed", nil
}

func (d *recordingAtomicDirectory) Lstat(name string) (os.FileInfo, error) {
	*d.events = append(*d.events, "lstat:"+name)
	if err := d.failures["lstat"]; err != nil {
		return nil, err
	}
	return nil, os.ErrNotExist
}

func (d *recordingAtomicDirectory) Rename(oldName, newName string) error {
	*d.events = append(*d.events, "rename:"+oldName+"->"+newName)
	return d.failures["rename"]
}

func (d *recordingAtomicDirectory) Remove(name string) error {
	*d.events = append(*d.events, "remove:"+name)
	return d.failures["remove"]
}

func (d *recordingAtomicDirectory) VerifyBound() error {
	d.verifyCalls++
	stage := "before"
	if d.verifyCalls > 1 {
		stage = "after"
	}
	key := "verify-parent-" + stage
	*d.events = append(*d.events, key)
	return d.failures[key]
}

func (d *recordingAtomicDirectory) Sync() error {
	*d.events = append(*d.events, "sync-dir")
	return d.failures["sync-dir"]
}

func (d *recordingAtomicDirectory) Close() error {
	*d.events = append(*d.events, "close-parent")
	return d.failures["close-parent"]
}

func recordingAtomicWriteOps(events *[]string, failures map[string]error) atomicWriteOps {
	if failures == nil {
		failures = make(map[string]error)
	}
	return atomicWriteOps{openParent: func(path string) (atomicWriteDirectory, error) {
		*events = append(*events, "open-parent:"+path)
		if err := failures["open-parent"]; err != nil {
			return nil, err
		}
		return &recordingAtomicDirectory{events: events, failures: failures}, nil
	}}
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
