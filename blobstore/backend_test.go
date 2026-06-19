package blobstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

// TestSaveStream_RoundTrip 验证流式保存大内容后可经 OpenReader 完整读回。
func TestSaveStream_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	// 构造 1MB 内容，验证流式路径不依赖一次性读入内存的行为正确。
	data := bytes.Repeat([]byte("hexagon-blob-"), 1<<16) // ~832KB

	rel, err := s.SaveStream(context.Background(), bytes.NewReader(data), "bin")
	if err != nil {
		t.Fatalf("SaveStream: %v", err)
	}

	rc, err := s.OpenReader(rel)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("读回内容与写入不一致 (len got=%d want=%d)", len(got), len(data))
	}
}

// TestSaveStream_ContentAddressMatchesSaveBytes 验证流式与字节保存对同一内容得到同一相对路径（内容寻址一致、去重）。
func TestSaveStream_ContentAddressMatchesSaveBytes(t *testing.T) {
	s := newTestStore(t)
	data := []byte("identical-content")

	relBytes, err := s.SaveBytes(data, "txt")
	if err != nil {
		t.Fatalf("SaveBytes: %v", err)
	}
	relStream, err := s.SaveStream(context.Background(), bytes.NewReader(data), "txt")
	if err != nil {
		t.Fatalf("SaveStream: %v", err)
	}
	if relBytes != relStream {
		t.Errorf("同内容相对路径应一致: SaveBytes=%q SaveStream=%q", relBytes, relStream)
	}
}

// TestSaveStream_Empty 验证空内容返回错误。
func TestSaveStream_Empty(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.SaveStream(context.Background(), strings.NewReader(""), "bin"); err == nil {
		t.Error("空内容应返回错误")
	}
}

// TestSaveStream_ContextCanceled 验证已取消的 ctx 会中断流式保存。
func TestSaveStream_ContextCanceled(t *testing.T) {
	s := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	if _, err := s.SaveStream(ctx, strings.NewReader("data"), "bin"); !errors.Is(err, context.Canceled) {
		t.Errorf("取消的 ctx 应返回 context.Canceled, got %v", err)
	}
}

// TestSaveStream_ConcurrentSameContent 验证并发流式保存同一内容不损坏、返回同一相对路径（内容寻址幂等）。
func TestSaveStream_ConcurrentSameContent(t *testing.T) {
	s := newTestStore(t)
	data := bytes.Repeat([]byte("concurrent-stream"), 4096)

	const n = 16
	rels := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rels[i], errs[i] = s.SaveStream(context.Background(), bytes.NewReader(data), "bin")
		}()
	}
	wg.Wait()

	for i := range n {
		if errs[i] != nil {
			t.Fatalf("并发 SaveStream %d: %v", i, errs[i])
		}
		if rels[i] != rels[0] {
			t.Errorf("同内容应得同一相对路径: [%d]=%q [0]=%q", i, rels[i], rels[0])
		}
	}
	// 内容可正确读回（未被并发写损坏）
	rc, err := s.OpenReader(rels[0])
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if !bytes.Equal(got, data) {
		t.Errorf("并发写后内容损坏 (len=%d want=%d)", len(got), len(data))
	}
}

// TestBlobstore_InterfaceSatisfied 验证 *Store 满足 Blobstore 抽象接口（运行期再确认编译期断言）。
func TestBlobstore_InterfaceSatisfied(t *testing.T) {
	var bs Blobstore = newTestStore(t)
	rel, err := bs.SaveBytes([]byte("x"), "bin")
	if err != nil {
		t.Fatalf("经接口 SaveBytes: %v", err)
	}
	if rel == "" {
		t.Error("应返回非空相对路径")
	}
}
