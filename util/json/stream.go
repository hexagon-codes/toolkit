package json

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

var (
	// ErrStreamClosed 流已关闭
	ErrStreamClosed = errors.New("json stream: closed")
	// ErrInvalidJSON 无效的 JSON
	ErrInvalidJSON = errors.New("json stream: invalid json")
)

// ============== 流式 JSON 解码器 ==============

// StreamDecoder 流式 JSON 解码器
// 用于从流中逐个读取 JSON 对象
type StreamDecoder struct {
	reader  *bufio.Reader
	decoder *json.Decoder
	closed  bool
}

// NewStreamDecoder 创建流式 JSON 解码器
func NewStreamDecoder(r io.Reader) *StreamDecoder {
	return &StreamDecoder{
		reader:  bufio.NewReader(r),
		decoder: json.NewDecoder(r),
	}
}

// NewStreamDecoderWithSize 创建指定缓冲区大小的流式 JSON 解码器
func NewStreamDecoderWithSize(r io.Reader, size int) *StreamDecoder {
	br := bufio.NewReaderSize(r, size)
	return &StreamDecoder{
		reader:  br,
		decoder: json.NewDecoder(br),
	}
}

// Decode 解码下一个 JSON 对象
func (d *StreamDecoder) Decode(v any) error {
	if d.closed {
		return ErrStreamClosed
	}
	return d.decoder.Decode(v)
}

// More 是否还有更多 JSON 对象
func (d *StreamDecoder) More() bool {
	if d.closed {
		return false
	}
	return d.decoder.More()
}

// Close 标记解码器为已关闭
func (d *StreamDecoder) Close() {
	d.closed = true
}

// ============== NDJSON（Newline Delimited JSON）解码器 ==============

// NDJSONDecoder NDJSON 解码器
// 用于解析每行一个 JSON 对象的格式
type NDJSONDecoder struct {
	scanner *bufio.Scanner
	closed  bool
	lastErr error
}

// NewNDJSONDecoder 创建 NDJSON 解码器
func NewNDJSONDecoder(r io.Reader) *NDJSONDecoder {
	return &NDJSONDecoder{
		scanner: bufio.NewScanner(r),
	}
}

// NewNDJSONDecoderWithSize 创建指定缓冲区大小的 NDJSON 解码器
func NewNDJSONDecoderWithSize(r io.Reader, size int) *NDJSONDecoder {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, size), size)
	return &NDJSONDecoder{
		scanner: scanner,
	}
}

// Decode 解码下一行 JSON
func (d *NDJSONDecoder) Decode(v any) error {
	if d.closed {
		return ErrStreamClosed
	}

	if !d.scanner.Scan() {
		if err := d.scanner.Err(); err != nil {
			d.lastErr = err
			return err
		}
		return io.EOF
	}

	line := d.scanner.Bytes()
	// 跳过空行
	if len(bytes.TrimSpace(line)) == 0 {
		return d.Decode(v) // 递归读取下一行
	}

	return json.Unmarshal(line, v)
}

// More 是否还有更多行
func (d *NDJSONDecoder) More() bool {
	return !d.closed && d.lastErr == nil
}

// Close 标记解码器为已关闭
func (d *NDJSONDecoder) Close() {
	d.closed = true
}

// Err 返回最后一个错误
func (d *NDJSONDecoder) Err() error {
	return d.lastErr
}

// ============== 流式 JSON 编码器 ==============

// StreamEncoder 流式 JSON 编码器
type StreamEncoder struct {
	writer  io.Writer
	encoder *json.Encoder
	closed  bool
}

// NewStreamEncoder 创建流式 JSON 编码器
func NewStreamEncoder(w io.Writer) *StreamEncoder {
	return &StreamEncoder{
		writer:  w,
		encoder: json.NewEncoder(w),
	}
}

// Encode 编码 JSON 对象
func (e *StreamEncoder) Encode(v any) error {
	if e.closed {
		return ErrStreamClosed
	}
	return e.encoder.Encode(v)
}

// SetIndent 设置缩进
func (e *StreamEncoder) SetIndent(prefix, indent string) {
	e.encoder.SetIndent(prefix, indent)
}

// SetEscapeHTML 设置是否转义 HTML
func (e *StreamEncoder) SetEscapeHTML(on bool) {
	e.encoder.SetEscapeHTML(on)
}

// Close 标记编码器为已关闭
func (e *StreamEncoder) Close() {
	e.closed = true
}

// ============== NDJSON 编码器 ==============

// NDJSONEncoder NDJSON 编码器
type NDJSONEncoder struct {
	writer io.Writer
	closed bool
}

// NewNDJSONEncoder 创建 NDJSON 编码器
func NewNDJSONEncoder(w io.Writer) *NDJSONEncoder {
	return &NDJSONEncoder{
		writer: w,
	}
}

// Encode 编码 JSON 对象并追加换行符
func (e *NDJSONEncoder) Encode(v any) error {
	if e.closed {
		return ErrStreamClosed
	}

	data, err := json.Marshal(v)
	if err != nil {
		return err
	}

	_, err = e.writer.Write(append(data, '\n'))
	return err
}

// Close 标记编码器为已关闭
func (e *NDJSONEncoder) Close() {
	e.closed = true
}

// ============== SSE JSON 解码器（用于 AI API 流式响应）==============

// SSEJSONDecoder SSE 格式的 JSON 解码器
// 用于解析 "data: {...}" 格式的流式响应
type SSEJSONDecoder struct {
	scanner   *bufio.Scanner
	closed    bool
	lastErr   error
	doneToken string // 结束标记（如 "[DONE]"）
}

// NewSSEJSONDecoder 创建 SSE JSON 解码器
func NewSSEJSONDecoder(r io.Reader) *SSEJSONDecoder {
	return &SSEJSONDecoder{
		scanner:   bufio.NewScanner(r),
		doneToken: "[DONE]",
	}
}

// NewSSEJSONDecoderWithDone 创建带自定义结束标记的 SSE JSON 解码器
func NewSSEJSONDecoderWithDone(r io.Reader, doneToken string) *SSEJSONDecoder {
	return &SSEJSONDecoder{
		scanner:   bufio.NewScanner(r),
		doneToken: doneToken,
	}
}

// Decode 解码下一个 SSE JSON 对象
func (d *SSEJSONDecoder) Decode(v any) error {
	if d.closed {
		return ErrStreamClosed
	}

	for d.scanner.Scan() {
		line := d.scanner.Text()

		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		// 解析 data: 前缀
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			data = strings.TrimSpace(data)

			// 检查结束标记
			if data == d.doneToken {
				return io.EOF
			}

			// 跳过空数据
			if data == "" {
				continue
			}

			return json.Unmarshal([]byte(data), v)
		}

		// 跳过其他 SSE 字段（event:, id:, retry:）
	}

	if err := d.scanner.Err(); err != nil {
		d.lastErr = err
		return err
	}

	return io.EOF
}

// More 是否还有更多数据
func (d *SSEJSONDecoder) More() bool {
	return !d.closed && d.lastErr == nil
}

// Close 标记解码器为已关闭
func (d *SSEJSONDecoder) Close() {
	d.closed = true
}

// Err 返回最后一个错误
func (d *SSEJSONDecoder) Err() error {
	return d.lastErr
}

// ============== 便捷函数 ==============

// DecodeStream 从流中解码所有 JSON 对象
func DecodeStream[T any](r io.Reader) ([]T, error) {
	decoder := json.NewDecoder(r)
	var results []T

	for decoder.More() {
		var item T
		if err := decoder.Decode(&item); err != nil {
			return results, err
		}
		results = append(results, item)
	}

	return results, nil
}

// DecodeNDJSON 从 NDJSON 流中解码所有对象
func DecodeNDJSON[T any](r io.Reader) ([]T, error) {
	decoder := NewNDJSONDecoder(r)
	var results []T

	for {
		var item T
		err := decoder.Decode(&item)
		if err == io.EOF {
			break
		}
		if err != nil {
			return results, err
		}
		results = append(results, item)
	}

	return results, nil
}

// DecodeSSEJSON 从 SSE 流中解码所有 JSON 对象
func DecodeSSEJSON[T any](r io.Reader) ([]T, error) {
	decoder := NewSSEJSONDecoder(r)
	var results []T

	for {
		var item T
		err := decoder.Decode(&item)
		if err == io.EOF {
			break
		}
		if err != nil {
			return results, err
		}
		results = append(results, item)
	}

	return results, nil
}

// EncodeNDJSON 将对象切片编码为 NDJSON
func EncodeNDJSON[T any](items []T) ([]byte, error) {
	var buf bytes.Buffer
	encoder := NewNDJSONEncoder(&buf)

	for _, item := range items {
		if err := encoder.Encode(item); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

// ============== 迭代器模式 ==============

// StreamIterator JSON 流迭代器
type StreamIterator[T any] struct {
	decoder *json.Decoder
	current T
	err     error
	done    bool
}

// NewStreamIterator 创建流迭代器
func NewStreamIterator[T any](r io.Reader) *StreamIterator[T] {
	return &StreamIterator[T]{
		decoder: json.NewDecoder(r),
	}
}

// Next 读取下一个元素
func (it *StreamIterator[T]) Next() bool {
	if it.done {
		return false
	}

	if !it.decoder.More() {
		it.done = true
		return false
	}

	var item T
	if err := it.decoder.Decode(&item); err != nil {
		it.err = err
		it.done = true
		return false
	}

	it.current = item
	return true
}

// Value 返回当前元素
func (it *StreamIterator[T]) Value() T {
	return it.current
}

// Err 返回错误
func (it *StreamIterator[T]) Err() error {
	return it.err
}

// NDJSONIterator NDJSON 流迭代器
type NDJSONIterator[T any] struct {
	decoder *NDJSONDecoder
	current T
	err     error
	done    bool
}

// NewNDJSONIterator 创建 NDJSON 迭代器
func NewNDJSONIterator[T any](r io.Reader) *NDJSONIterator[T] {
	return &NDJSONIterator[T]{
		decoder: NewNDJSONDecoder(r),
	}
}

// Next 读取下一个元素
func (it *NDJSONIterator[T]) Next() bool {
	if it.done {
		return false
	}

	var item T
	err := it.decoder.Decode(&item)
	if err == io.EOF {
		it.done = true
		return false
	}
	if err != nil {
		it.err = err
		it.done = true
		return false
	}

	it.current = item
	return true
}

// Value 返回当前元素
func (it *NDJSONIterator[T]) Value() T {
	return it.current
}

// Err 返回错误
func (it *NDJSONIterator[T]) Err() error {
	return it.err
}

// SSEJSONIterator SSE JSON 流迭代器
type SSEJSONIterator[T any] struct {
	decoder *SSEJSONDecoder
	current T
	err     error
	done    bool
}

// NewSSEJSONIterator 创建 SSE JSON 迭代器
func NewSSEJSONIterator[T any](r io.Reader) *SSEJSONIterator[T] {
	return &SSEJSONIterator[T]{
		decoder: NewSSEJSONDecoder(r),
	}
}

// Next 读取下一个元素
func (it *SSEJSONIterator[T]) Next() bool {
	if it.done {
		return false
	}

	var item T
	err := it.decoder.Decode(&item)
	if err == io.EOF {
		it.done = true
		return false
	}
	if err != nil {
		it.err = err
		it.done = true
		return false
	}

	it.current = item
	return true
}

// Value 返回当前元素
func (it *SSEJSONIterator[T]) Value() T {
	return it.current
}

// Err 返回错误
func (it *SSEJSONIterator[T]) Err() error {
	return it.err
}
