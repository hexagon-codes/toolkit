package blobstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type closeErrorBody struct {
	io.Reader
	err error
}

type idleClosingRoundTripper struct {
	closed atomic.Int32
}

type gatedReader struct {
	entered chan struct{}
	release chan struct{}
	data    []byte
	sent    bool
}

func (r *gatedReader) Read(buffer []byte) (int, error) {
	if r.sent {
		return 0, io.EOF
	}
	r.sent = true
	close(r.entered)
	<-r.release
	return copy(buffer, r.data), nil
}

func (*idleClosingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("HTTP request is not expected")
}

func (t *idleClosingRoundTripper) CloseIdleConnections() {
	t.closed.Add(1)
}

func (b *closeErrorBody) Close() error {
	return b.err
}

func TestStoreOperationsRemainBoundToOpenedRootAfterPathRebind(t *testing.T) {
	parent := t.TempDir()
	rootPath := filepath.Join(parent, "store")
	store, err := NewStore(rootPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	registerStoreCleanup(t, store)

	existingContent := []byte("original content")
	existingPath, err := store.SaveBytes(existingContent, "bin")
	if err != nil {
		t.Fatalf("SaveBytes before rebind failed: %v", err)
	}
	expiredPath, err := store.SaveBytesWithTTL([]byte("expired content"), "bin", time.Nanosecond)
	if err != nil {
		t.Fatalf("SaveBytesWithTTL before rebind failed: %v", err)
	}
	expiredAt, ok, err := store.ExpiresAt(expiredPath)
	if err != nil || !ok {
		t.Fatalf("ExpiresAt before rebind = (_, %v, %v), want metadata", ok, err)
	}

	openedRootPath := filepath.Join(parent, "opened-root")
	if renameErr := os.Rename(rootPath, openedRootPath); renameErr != nil {
		t.Fatalf("rename opened root: %v", renameErr)
	}
	if mkdirErr := os.Mkdir(rootPath, 0o700); mkdirErr != nil {
		t.Fatalf("create replacement root: %v", mkdirErr)
	}
	replacementBlobPath := filepath.Join(rootPath, filepath.FromSlash(existingPath))
	if mkdirAllErr := os.MkdirAll(filepath.Dir(replacementBlobPath), 0o700); mkdirAllErr != nil {
		t.Fatalf("create replacement blob directory: %v", mkdirAllErr)
	}
	if writeErr := os.WriteFile(replacementBlobPath, []byte("replacement content"), 0o600); writeErr != nil {
		t.Fatalf("write replacement blob: %v", writeErr)
	}

	file, err := store.Open(existingPath)
	if err != nil {
		t.Fatalf("Open after rebind failed: %v", err)
	}
	got, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("read opened blob: read=%v close=%v", readErr, closeErr)
	}
	if !bytes.Equal(got, existingContent) {
		t.Fatalf("Open read rebound root content: got %q want %q", got, existingContent)
	}

	bytesPath, err := store.SaveBytes([]byte("bytes after rebind"), "bin")
	if err != nil {
		t.Fatalf("SaveBytes after rebind failed: %v", err)
	}
	assertBlobExistsOnlyInOpenedRoot(t, openedRootPath, rootPath, bytesPath)

	streamPath, err := store.SaveStream(context.Background(), bytes.NewReader([]byte("stream after rebind")), "bin")
	if err != nil {
		t.Fatalf("SaveStream after rebind failed: %v", err)
	}
	assertBlobExistsOnlyInOpenedRoot(t, openedRootPath, rootPath, streamPath)

	if setTTLErr := store.SetTTL(existingPath, time.Hour); setTTLErr != nil {
		t.Fatalf("SetTTL after rebind failed: %v", setTTLErr)
	}
	if _, ok, expiresErr := store.ExpiresAt(existingPath); expiresErr != nil || !ok {
		t.Fatalf("ExpiresAt after rebind = (_, %v, %v), want metadata", ok, expiresErr)
	}
	if _, statErr := os.Stat(filepath.Join(openedRootPath, filepath.FromSlash(existingPath)) + ttlSuffix); statErr != nil {
		t.Fatalf("TTL metadata missing from opened root: %v", statErr)
	}
	if _, replacementStatErr := os.Stat(replacementBlobPath + ttlSuffix); !os.IsNotExist(replacementStatErr) {
		t.Fatalf("TTL metadata reached replacement root: %v", replacementStatErr)
	}

	purged, err := store.Purge(expiredAt.Add(time.Nanosecond))
	if err != nil {
		t.Fatalf("Purge after rebind failed: %v", err)
	}
	if purged != 1 {
		t.Fatalf("Purge removed %d blobs, want 1", purged)
	}
	if _, err := os.Stat(filepath.Join(openedRootPath, filepath.FromSlash(expiredPath))); !os.IsNotExist(err) {
		t.Fatalf("expired blob remains in opened root: %v", err)
	}
}

func assertBlobExistsOnlyInOpenedRoot(t *testing.T, openedRootPath, replacementRootPath, relPath string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(openedRootPath, filepath.FromSlash(relPath))); err != nil {
		t.Fatalf("blob missing from opened root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(replacementRootPath, filepath.FromSlash(relPath))); !os.IsNotExist(err) {
		t.Fatalf("blob reached replacement root: %v", err)
	}
}

func TestStoreCloseRejectsAllOperationsWithErrStoreClosed(t *testing.T) {
	store := newTestStore(t)
	var requests atomic.Int32
	store.httpc.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("unexpected HTTP request")
	})

	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("repeated Close failed: %v", err)
	}

	operations := map[string]func() error{
		"SaveBytes": func() error {
			_, err := store.SaveBytes([]byte("closed"), "bin")
			return err
		},
		"SaveStream": func() error {
			_, err := store.SaveStream(context.Background(), bytes.NewReader([]byte("closed")), "bin")
			return err
		},
		"SaveFromURL": func() error {
			_, err := store.SaveFromURL(context.Background(), "https://example.invalid/blob", "bin")
			return err
		},
		"Open": func() error {
			_, err := store.Open("missing.bin")
			return err
		},
		"OpenReader": func() error {
			_, err := store.OpenReader("missing.bin")
			return err
		},
		"SetTTL": func() error {
			return store.SetTTL("missing.bin", time.Hour)
		},
		"SaveBytesWithTTL": func() error {
			_, err := store.SaveBytesWithTTL([]byte("closed"), "bin", time.Hour)
			return err
		},
		"ExpiresAt": func() error {
			_, _, err := store.ExpiresAt("missing.bin")
			return err
		},
		"Purge": func() error {
			_, err := store.Purge(time.Now())
			return err
		},
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			if err := operation(); !errors.Is(err, ErrStoreClosed) {
				t.Fatalf("operation error = %v, want ErrStoreClosed", err)
			}
		})
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("SaveFromURL issued %d requests after Close", got)
	}
}

func TestStoreCloseCanRetryAfterVisibleFailure(t *testing.T) {
	store := newTestStore(t)
	wantErr := errors.New("close root failed")
	actualClose := store.closeRoot
	var attempts atomic.Int32
	store.closeRoot = func(root *os.Root) error {
		if attempts.Add(1) == 1 {
			return wantErr
		}
		return actualClose(root)
	}

	if err := store.Close(); !errors.Is(err, wantErr) {
		t.Fatalf("first Close error = %v, want injected failure", err)
	}
	if _, err := store.SaveBytes([]byte("must stay closed"), "bin"); !errors.Is(err, ErrStoreClosed) {
		t.Fatalf("operation after failed Close = %v, want ErrStoreClosed", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("retry Close failed: %v", err)
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("close attempts = %d, want 2", got)
	}
}

func TestStoreConcurrentCloseIsSafe(t *testing.T) {
	store := newTestStore(t)
	const callers = 32
	start := make(chan struct{})
	errs := make(chan error, callers)
	var wait sync.WaitGroup
	wait.Add(callers)
	for range callers {
		go func() {
			defer wait.Done()
			<-start
			errs <- store.Close()
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Close failed: %v", err)
		}
	}
}

func TestStoreCloseWaitsForInflightOperation(t *testing.T) {
	store := newTestStore(t)
	reader := &gatedReader{
		entered: make(chan struct{}),
		release: make(chan struct{}),
		data:    []byte("inflight content"),
	}
	type saveResult struct {
		path string
		err  error
	}
	saveDone := make(chan saveResult, 1)
	go func() {
		path, err := store.SaveStream(context.Background(), reader, "bin")
		saveDone <- saveResult{path: path, err: err}
	}()
	<-reader.entered

	closeStarted := make(chan struct{})
	closeDone := make(chan error, 1)
	go func() {
		close(closeStarted)
		closeDone <- store.Close()
	}()
	<-closeStarted

	var earlyCloseErr error
	closedEarly := false
	select {
	case earlyCloseErr = <-closeDone:
		closedEarly = true
	case <-time.After(100 * time.Millisecond):
	}
	close(reader.release)

	saved := <-saveDone
	if closedEarly {
		t.Fatalf("Close returned before the in-flight operation completed: %v", earlyCloseErr)
	}
	if saved.err != nil || saved.path == "" {
		t.Fatalf("in-flight SaveStream = (%q, %v), want success", saved.path, saved.err)
	}
	if err := <-closeDone; err != nil {
		t.Fatalf("Close after in-flight operation failed: %v", err)
	}
}

func TestStoreCloseReleasesOwnedHTTPIdleConnections(t *testing.T) {
	store := newTestStore(t)
	transport := &idleClosingRoundTripper{}
	store.httpc.Transport = transport

	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("repeated Close failed: %v", err)
	}
	if got := transport.closed.Load(); got != 1 {
		t.Fatalf("CloseIdleConnections calls = %d, want 1", got)
	}
}

func TestSaveFromURLReturnsResponseBodyCloseError(t *testing.T) {
	store := newTestStore(t)
	wantErr := errors.New("response body close failed")
	store.httpc.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Body:          &closeErrorBody{Reader: bytes.NewReader([]byte("downloaded content")), err: wantErr},
			ContentLength: int64(len("downloaded content")),
		}, nil
	})

	relPath, err := store.SaveFromURL(context.Background(), "https://example.invalid/blob", "bin")
	if relPath == "" {
		t.Fatal("SaveFromURL did not return the published blob path")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("SaveFromURL error = %v, want response body close failure", err)
	}
	file, openErr := store.Open(relPath)
	if openErr != nil {
		t.Fatalf("published blob cannot be opened: %v", openErr)
	}
	if closeErr := file.Close(); closeErr != nil {
		t.Fatalf("close published blob: %v", closeErr)
	}
}
