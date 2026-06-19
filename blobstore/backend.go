// Package blobstore 提供内容寻址的 blob 存储。
//
// 本文件定义制品存储的抽象接口 Blobstore、本地 Store 的流式读写实现，
// 以及对象存储后端（S3/R2 等）的接口约定 ObjectBackend。
//
// 设计取舍：通用工具库不内置任何云厂商 SDK，避免给所有使用方拖入重依赖；
// 对象存储后端只定义接口（seam），具体实现由部署方在 infra 层按各家 SDK 适配。
package blobstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Blobstore 是制品存储的抽象接口：内容寻址保存（字节/流）、流式读取、TTL 与清理。
//
// 本地文件系统实现见 *Store；对象存储（S3/R2）后端经 ObjectBackend 适配。
type Blobstore interface {
	// SaveBytes 保存字节内容，返回内容寻址的相对路径。
	SaveBytes(data []byte, ext string) (relPath string, err error)
	// SaveStream 流式保存（适合大文件，内存占用有界），返回内容寻址的相对路径。
	SaveStream(ctx context.Context, r io.Reader, ext string) (relPath string, err error)
	// OpenReader 流式读取已保存的内容（带路径穿越防护）。
	OpenReader(relPath string) (io.ReadCloser, error)
	// SetTTL 为指定内容设置存活时长，到期后可被 Purge 清理。
	SetTTL(relPath string, ttl time.Duration) error
	// Purge 清理在 now 之前过期的内容，返回清理数量。
	Purge(now time.Time) (removed int, err error)
}

// 编译期断言：本地 Store 满足 Blobstore 抽象接口。
var _ Blobstore = (*Store)(nil)

// SaveStream 流式保存内容并返回内容寻址的相对路径（如 "202604/abc...mp4"）。
//
// 与 SaveBytes 同样按 SHA-256 内容寻址 + 同内容去重 + 原子落盘，但不要求把整个
// 内容读进内存：边读边写临时文件、同时计算哈希，读完后按哈希 rename 到最终路径。
// 适合视频等大文件。ext 不带点；空内容返回错误；ctx 取消会中断读取。
func (s *Store) SaveStream(ctx context.Context, r io.Reader, ext string) (string, error) {
	if ext == "" {
		ext = "bin"
	}
	ext = strings.TrimPrefix(ext, ".")

	// 临时文件落在根目录下，确保与最终路径同一文件系统（rename 原子）。
	tmp, err := os.CreateTemp(s.root, ".stream.*.tmp")
	if err != nil {
		return "", fmt.Errorf("create tmp: %w", err)
	}
	tmpPath := tmp.Name()
	cleanupTmp := func() { _ = os.Remove(tmpPath) }

	h := sha256.New()
	// 边写临时文件边计算哈希；ctxReader 让取消能中断长时间拷贝。
	n, err := io.Copy(io.MultiWriter(tmp, h), &ctxReader{ctx: ctx, r: r})
	if err != nil {
		_ = tmp.Close()
		cleanupTmp()
		return "", fmt.Errorf("stream copy: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanupTmp()
		return "", fmt.Errorf("close tmp %s: %w", tmpPath, err)
	}
	if n == 0 {
		cleanupTmp()
		return "", fmt.Errorf("empty data")
	}

	hash := hex.EncodeToString(h.Sum(nil))
	subdir := time.Now().Format("200601") // YYYYMM
	relPath := filepath.Join(subdir, hash+"."+ext)
	abs := filepath.Join(s.root, relPath)

	// 同内容已存在则丢弃临时文件、复用既有。
	if _, statErr := os.Stat(abs); statErr == nil {
		cleanupTmp()
		return filepath.ToSlash(relPath), nil
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		cleanupTmp()
		return "", fmt.Errorf("mkdir: %w", err)
	}
	if err := os.Rename(tmpPath, abs); err != nil {
		cleanupTmp()
		return "", fmt.Errorf("rename %s→%s: %w", tmpPath, abs, err)
	}
	return filepath.ToSlash(relPath), nil
}

// OpenReader 流式读取已保存的内容，返回 io.ReadCloser（带路径穿越防护）。
//
// 与 Open 等价但返回接口类型，便于满足 Blobstore 抽象。调用方负责 Close。
func (s *Store) OpenReader(relPath string) (io.ReadCloser, error) {
	f, err := s.Open(relPath)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// ctxReader 包装 io.Reader，使每次 Read 前先检查 ctx 取消，
// 让大文件流式拷贝可被取消/超时中断。
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// ObjectBackend 是对象存储后端（S3 / Cloudflare R2 / GCS 等）的接口约定。
//
// 通用工具库只定义此 seam，不内置任何云厂商 SDK；具体后端实现（依赖各家 SDK、
// 凭据与区域配置）由部署方在 infra 层提供，并可与本地 Store 经统一上层封装并存。
// key 为对象键（如内容寻址相对路径）。
type ObjectBackend interface {
	// PutObject 上传对象；size<0 表示长度未知（由实现决定是否分片）。
	PutObject(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	// GetObject 下载对象，返回流；调用方负责 Close。
	GetObject(ctx context.Context, key string) (io.ReadCloser, error)
	// DeleteObject 删除对象。
	DeleteObject(ctx context.Context, key string) error
	// StatObject 查询对象是否存在及大小。
	StatObject(ctx context.Context, key string) (exists bool, size int64, err error)
}
