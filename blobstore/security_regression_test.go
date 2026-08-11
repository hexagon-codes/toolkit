package blobstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPurgeDoesNotTreatTTLExtensionAsMetadata(t *testing.T) {
	store := newTestStore(t)
	relPath, err := store.SaveBytes([]byte("0"), "ttl")
	if err != nil {
		t.Fatalf("SaveBytes failed: %v", err)
	}

	purged, err := store.Purge(time.Now())
	if err != nil {
		t.Fatalf("Purge failed: %v", err)
	}
	if purged != 0 {
		t.Fatalf("Purge removed %d blobs; want 0", purged)
	}
	file, err := store.Open(relPath)
	if err != nil {
		t.Fatalf("permanent .ttl blob was removed: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close blob: %v", err)
	}
}

func TestSetTTLReplacesMetadataAtomically(t *testing.T) {
	store := newTestStore(t)
	relPath, err := store.SaveBytes([]byte("ttl metadata"), "bin")
	if err != nil {
		t.Fatalf("SaveBytes failed: %v", err)
	}
	if setTTLErr := store.SetTTL(relPath, time.Hour); setTTLErr != nil {
		t.Fatalf("first SetTTL failed: %v", setTTLErr)
	}

	metadataPath := filepath.Join(store.Root(), filepath.FromSlash(relPath)) + ttlSuffix
	previous, err := os.Open(metadataPath)
	if err != nil {
		t.Fatalf("open first TTL metadata: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := previous.Close(); closeErr != nil {
			t.Errorf("close first TTL metadata: %v", closeErr)
		}
	})
	before, err := previous.Stat()
	if err != nil {
		t.Fatalf("stat first TTL metadata: %v", err)
	}
	if setTTLErr := store.SetTTL(relPath, 2*time.Hour); setTTLErr != nil {
		t.Fatalf("second SetTTL failed: %v", setTTLErr)
	}
	after, err := os.Stat(metadataPath)
	if err != nil {
		t.Fatalf("stat second TTL metadata: %v", err)
	}
	if os.SameFile(before, after) {
		t.Fatal("SetTTL updated metadata in place; want atomic replacement")
	}
	oldMetadata, err := io.ReadAll(previous)
	if err != nil {
		t.Fatalf("read first TTL metadata: %v", err)
	}
	newMetadata, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("read second TTL metadata: %v", err)
	}
	if bytes.Equal(oldMetadata, newMetadata) {
		t.Fatalf("TTL metadata content was not replaced: %q", newMetadata)
	}
}

func TestSaveBytesRepairsCorruptedContentAtAddress(t *testing.T) {
	store := newTestStore(t)
	want := []byte("content-addressed payload")
	relPath, err := store.SaveBytes(want, "bin")
	if err != nil {
		t.Fatalf("first SaveBytes failed: %v", err)
	}
	absolutePath := filepath.Join(store.Root(), filepath.FromSlash(relPath))
	if writeErr := os.WriteFile(absolutePath, []byte("corrupted"), 0o600); writeErr != nil {
		t.Fatalf("corrupt stored blob: %v", writeErr)
	}

	returnedPath, err := store.SaveBytes(want, "bin")
	if err != nil {
		t.Fatalf("second SaveBytes failed: %v", err)
	}
	if returnedPath != relPath {
		t.Fatalf("second SaveBytes path = %q, want %q", returnedPath, relPath)
	}
	got, err := os.ReadFile(absolutePath)
	if err != nil {
		t.Fatalf("read repaired blob: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("stored content = %q, want %q", got, want)
	}
}

func TestSaveStreamRepairsCorruptedContentAtAddress(t *testing.T) {
	store := newTestStore(t)
	want := []byte("streamed content-addressed payload")
	relPath, err := store.SaveStream(t.Context(), bytes.NewReader(want), "bin")
	if err != nil {
		t.Fatalf("first SaveStream failed: %v", err)
	}
	absolutePath := filepath.Join(store.Root(), filepath.FromSlash(relPath))
	if writeErr := os.WriteFile(absolutePath, []byte("corrupted"), 0o600); writeErr != nil {
		t.Fatalf("corrupt stored blob: %v", writeErr)
	}

	returnedPath, err := store.SaveStream(t.Context(), bytes.NewReader(want), "bin")
	if err != nil {
		t.Fatalf("second SaveStream failed: %v", err)
	}
	if returnedPath != relPath {
		t.Fatalf("second SaveStream path = %q, want %q", returnedPath, relPath)
	}
	got, err := os.ReadFile(absolutePath)
	if err != nil {
		t.Fatalf("read repaired blob: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("stored content = %q, want %q", got, want)
	}
}

func TestSetTTLRejectsUnrepresentableExpiry(t *testing.T) {
	store := newTestStore(t)
	relPath, err := store.SaveBytes([]byte("long ttl"), "bin")
	if err != nil {
		t.Fatalf("SaveBytes failed: %v", err)
	}

	if err := store.SetTTL(relPath, time.Duration(math.MaxInt64)); err == nil {
		t.Fatal("SetTTL accepted an expiry beyond UnixNano range")
	}
	if _, ok, err := store.ExpiresAt(relPath); err != nil || ok {
		t.Fatalf("ExpiresAt after rejected TTL = (_, %v, %v), want no metadata", ok, err)
	}
}

func TestNewStoreRejectsFilesystemRoot(t *testing.T) {
	root := string(os.PathSeparator)
	if runtime.GOOS == "windows" {
		root = filepath.VolumeName(os.TempDir()) + string(os.PathSeparator)
	}

	if store, err := NewStore(root); err == nil {
		registerStoreCleanup(t, store)
		t.Fatalf("NewStore(%q) = %#v, want error", root, store)
	}
}

func TestNewStoreRejectsEmptyRoot(t *testing.T) {
	if store, err := NewStore(""); err == nil {
		registerStoreCleanup(t, store)
		t.Fatalf("NewStore with an empty root = %#v, want error", store)
	}
}

func TestNewStoreTightensExistingRootPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX permission semantics")
	}

	root := filepath.Join(t.TempDir(), "store")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatalf("create existing root: %v", err)
	}
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	registerStoreCleanup(t, store)
	info, err := os.Stat(root)
	if err != nil {
		t.Fatalf("stat root: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("existing root permissions = %04o, want 0700", got)
	}
}

func TestOpenRejectsDirectory(t *testing.T) {
	store := newTestStore(t)
	if err := os.Mkdir(filepath.Join(store.Root(), "directory"), 0o700); err != nil {
		t.Fatalf("create directory: %v", err)
	}

	file, err := store.Open("directory")
	if err == nil {
		_ = file.Close()
		t.Fatal("Open accepted a directory as a blob")
	}
}

func TestStreamingAPIsRejectNilInputsWithoutPanicking(t *testing.T) {
	store := newTestStore(t)
	var typedNilReader *bytes.Reader
	var nilContext context.Context
	tests := map[string]func() error{
		"stream context": func() error {
			_, err := store.SaveStream(nilContext, bytes.NewReader([]byte("content")), "bin")
			return err
		},
		"stream reader": func() error {
			_, err := store.SaveStream(context.Background(), nil, "bin")
			return err
		},
		"typed nil stream reader": func() error {
			_, err := store.SaveStream(context.Background(), typedNilReader, "bin")
			return err
		},
		"download context": func() error {
			_, err := store.SaveFromURL(nilContext, "https://example.invalid/blob", "bin")
			return err
		},
	}

	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("API panicked for nil input: %v", recovered)
				}
			}()
			if err := call(); err == nil {
				t.Fatal("API accepted nil input")
			}
		})
	}
}

func TestLimitedDownloadReaderRejectsOversizedContent(t *testing.T) {
	reader := newMaxBytesReader(bytes.NewReader([]byte("12345")), 4)
	data, err := io.ReadAll(reader)
	if !errors.Is(err, errRemoteBlobTooLarge) {
		t.Fatalf("ReadAll error = %v, want remote blob size error", err)
	}
	if string(data) != "1234" {
		t.Fatalf("ReadAll data = %q, want only permitted bytes", data)
	}
}

func TestSetTTLAndPurgeDoNotLoseSuccessfulRenewal(t *testing.T) {
	store := newTestStore(t)
	for attempt := range 5 {
		relPath, err := store.SaveBytes([]byte("ttl-race-"+strconv.Itoa(attempt)), "bin")
		if err != nil {
			t.Fatalf("SaveBytes failed: %v", err)
		}
		if setTTLErr := store.SetTTL(relPath, time.Nanosecond); setTTLErr != nil {
			t.Fatalf("SetTTL expired value failed: %v", setTTLErr)
		}
		time.Sleep(time.Microsecond)

		start := make(chan struct{})
		var successfulRenewals atomic.Int64
		var writers sync.WaitGroup
		for range 16 {
			writers.Add(1)
			go func() {
				defer writers.Done()
				<-start
				if store.SetTTL(relPath, time.Hour) == nil {
					successfulRenewals.Add(1)
				}
			}()
		}
		close(start)
		_, purgeErr := store.Purge(time.Now())
		writers.Wait()
		if purgeErr != nil {
			t.Fatalf("Purge failed: %v", purgeErr)
		}
		if successfulRenewals.Load() == 0 {
			continue
		}
		file, err := store.Open(relPath)
		if err != nil {
			t.Fatalf("attempt %d lost a blob after %d successful TTL renewals: %v", attempt, successfulRenewals.Load(), err)
		}
		if err := file.Close(); err != nil {
			t.Fatalf("close blob: %v", err)
		}
	}
}

func TestIndependentStoresDoNotLoseSuccessfulTTLRenewal(t *testing.T) {
	purgeStore := newTestStore(t)
	renewStore, err := NewStore(purgeStore.Root())
	if err != nil {
		t.Fatalf("NewStore for shared root failed: %v", err)
	}
	registerStoreCleanup(t, renewStore)

	for attempt := range 5 {
		relPath, err := purgeStore.SaveBytes([]byte("shared-ttl-race-"+strconv.Itoa(attempt)), "bin")
		if err != nil {
			t.Fatalf("SaveBytes failed: %v", err)
		}
		if setTTLErr := purgeStore.SetTTL(relPath, time.Nanosecond); setTTLErr != nil {
			t.Fatalf("SetTTL expired value failed: %v", setTTLErr)
		}
		time.Sleep(time.Microsecond)

		start := make(chan struct{})
		var successfulRenewals atomic.Int64
		var writers sync.WaitGroup
		for range 16 {
			writers.Add(1)
			go func() {
				defer writers.Done()
				<-start
				if renewStore.SetTTL(relPath, time.Hour) == nil {
					successfulRenewals.Add(1)
				}
			}()
		}
		close(start)
		_, purgeErr := purgeStore.Purge(time.Now())
		writers.Wait()
		if purgeErr != nil {
			t.Fatalf("Purge failed: %v", purgeErr)
		}
		if successfulRenewals.Load() == 0 {
			continue
		}
		file, err := purgeStore.Open(relPath)
		if err != nil {
			t.Fatalf("attempt %d lost a blob after %d successful cross-store TTL renewals: %v", attempt, successfulRenewals.Load(), err)
		}
		if err := file.Close(); err != nil {
			t.Fatalf("close blob: %v", err)
		}
	}
}
