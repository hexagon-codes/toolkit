package json

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
)

const (
	// DefaultMaxRecordBytes 是 NDJSON 和 SSE 单条记录的默认上限。
	DefaultMaxRecordBytes = 1 << 20
	// DefaultMaxCollectionItems 是聚合便捷函数返回的最大元素数。
	DefaultMaxCollectionItems = 100_000
)

var (
	// ErrStreamClosed 流已关闭
	ErrStreamClosed = errors.New("json stream: closed")
	// ErrInvalidJSON 无效的 JSON
	ErrInvalidJSON = errors.New("json: invalid input")
	// ErrInvalidReader 输入读取器无效
	ErrInvalidReader = errors.New("json stream: invalid reader")
	// ErrInvalidWriter 输出写入器无效
	ErrInvalidWriter = errors.New("json stream: invalid writer")
	// ErrInvalidSize 大小限制无效
	ErrInvalidSize = errors.New("json stream: invalid size")
	// ErrRecordTooLarge 单条记录超过限制
	ErrRecordTooLarge = errors.New("json stream: record too large")
	// ErrTooManyItems 聚合结果超过元素数量限制
	ErrTooManyItems = errors.New("json stream: too many items")
)

// ============== 流式 JSON 解码器 ==============

// StreamDecoder 流式 JSON 解码器
// 用于从流中逐个读取 JSON 对象
type StreamDecoder struct {
	reader  *bufio.Reader
	decoder *json.Decoder
	closed  bool
	lastErr error
	initErr error
}

// NewStreamDecoder 创建流式 JSON 解码器
func NewStreamDecoder(r io.Reader) *StreamDecoder {
	return newStreamDecoder(r, DefaultMaxDocumentBytes)
}

// NewStreamDecoderWithSize 创建指定最大输入字节数的流式 JSON 解码器。
func NewStreamDecoderWithSize(r io.Reader, size int) *StreamDecoder {
	return newStreamDecoder(r, size)
}

func newStreamDecoder(r io.Reader, size int) *StreamDecoder {
	if isNilInterface(r) {
		return &StreamDecoder{initErr: ErrInvalidReader}
	}
	if size <= 0 {
		return &StreamDecoder{initErr: ErrInvalidSize}
	}
	br := bufio.NewReader(newBoundedReader(r, size, ErrDocumentTooLarge))
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
	if d.initErr != nil {
		return d.initErr
	}
	var raw json.RawMessage
	if err := d.decoder.Decode(&raw); err != nil {
		if err != io.EOF {
			d.lastErr = classifyDecodeError(err)
			return d.lastErr
		}
		return io.EOF
	}
	if err := decodeJSONBytes(raw, v); err != nil {
		d.lastErr = err
		return err
	}
	return nil
}

// More 是否还有更多 JSON 对象
func (d *StreamDecoder) More() bool {
	if d.closed || d.initErr != nil || d.lastErr != nil {
		return false
	}
	return d.decoder.More()
}

// Close 标记解码器为已关闭
func (d *StreamDecoder) Close() {
	d.closed = true
}

// Err 返回最后一个非 EOF 错误。
func (d *StreamDecoder) Err() error {
	return d.lastErr
}

// ============== NDJSON（Newline Delimited JSON）解码器 ==============

// NDJSONDecoder NDJSON 解码器
// 用于解析每行一个 JSON 对象的格式
type NDJSONDecoder struct {
	scanner        *bufio.Scanner
	maxRecordBytes int
	closed         bool
	lastErr        error
	initErr        error
	done           bool
}

// NewNDJSONDecoder 创建 NDJSON 解码器
func NewNDJSONDecoder(r io.Reader) *NDJSONDecoder {
	return newNDJSONDecoder(r, DefaultMaxRecordBytes)
}

// NewNDJSONDecoderWithSize 创建指定最大记录字节数的 NDJSON 解码器。
func NewNDJSONDecoderWithSize(r io.Reader, size int) *NDJSONDecoder {
	return newNDJSONDecoder(r, size)
}

func newNDJSONDecoder(r io.Reader, size int) *NDJSONDecoder {
	if isNilInterface(r) {
		return &NDJSONDecoder{initErr: ErrInvalidReader}
	}
	if size <= 0 {
		return &NDJSONDecoder{initErr: ErrInvalidSize}
	}
	scanner := newLineScanner(r, size)
	return &NDJSONDecoder{
		scanner:        scanner,
		maxRecordBytes: size,
	}
}

// Decode 解码下一行 JSON
func (d *NDJSONDecoder) Decode(v any) error {
	if d.closed {
		return ErrStreamClosed
	}
	if d.initErr != nil {
		return d.initErr
	}
	if d.done {
		return io.EOF
	}

	for {
		if !d.scanner.Scan() {
			if err := d.scanner.Err(); err != nil {
				d.lastErr = classifyScannerError(err)
				return d.lastErr
			}
			d.done = true
			return io.EOF
		}

		line := d.scanner.Bytes()
		if len(line) > d.maxRecordBytes {
			d.lastErr = fmt.Errorf("%w: limit is %d bytes", ErrRecordTooLarge, d.maxRecordBytes)
			return d.lastErr
		}
		// 跳过空行（使用循环代替递归，避免大量空行导致栈溢出）
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		if err := decodeJSONBytes(line, v); err != nil {
			d.lastErr = err
			return err
		}
		return nil
	}
}

// More 是否还有更多行
func (d *NDJSONDecoder) More() bool {
	return !d.closed && !d.done && d.initErr == nil && d.lastErr == nil
}

// Close 标记解码器为已关闭
func (d *NDJSONDecoder) Close() {
	d.closed = true
	d.done = true
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
	initErr error
}

// NewStreamEncoder 创建流式 JSON 编码器
func NewStreamEncoder(w io.Writer) *StreamEncoder {
	if isNilInterface(w) {
		return &StreamEncoder{writer: io.Discard, encoder: json.NewEncoder(io.Discard), initErr: ErrInvalidWriter}
	}
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
	if e.initErr != nil {
		return e.initErr
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
	writer  io.Writer
	closed  bool
	initErr error
}

// NewNDJSONEncoder 创建 NDJSON 编码器
func NewNDJSONEncoder(w io.Writer) *NDJSONEncoder {
	if isNilInterface(w) {
		return &NDJSONEncoder{writer: io.Discard, initErr: ErrInvalidWriter}
	}
	return &NDJSONEncoder{
		writer: w,
	}
}

// Encode 编码 JSON 对象并追加换行符
func (e *NDJSONEncoder) Encode(v any) error {
	if e.closed {
		return ErrStreamClosed
	}
	if e.initErr != nil {
		return e.initErr
	}

	data, err := json.Marshal(v)
	if err != nil {
		return err
	}

	data = append(data, '\n')
	written, err := e.writer.Write(data)
	if err != nil {
		return err
	}
	if written != len(data) {
		return io.ErrShortWrite
	}
	return nil
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
	initErr   error
	done      bool
}

// NewSSEJSONDecoder 创建 SSE JSON 解码器
func NewSSEJSONDecoder(r io.Reader) *SSEJSONDecoder {
	return newSSEJSONDecoder(r, "[DONE]")
}

// NewSSEJSONDecoderWithDone 创建带自定义结束标记的 SSE JSON 解码器
func NewSSEJSONDecoderWithDone(r io.Reader, doneToken string) *SSEJSONDecoder {
	return newSSEJSONDecoder(r, doneToken)
}

func newSSEJSONDecoder(r io.Reader, doneToken string) *SSEJSONDecoder {
	if isNilInterface(r) {
		return &SSEJSONDecoder{doneToken: doneToken, initErr: ErrInvalidReader}
	}
	return &SSEJSONDecoder{
		scanner:   newLineScanner(r, DefaultMaxRecordBytes),
		doneToken: doneToken,
	}
}

// Decode 解码下一个 SSE JSON 对象
func (d *SSEJSONDecoder) Decode(v any) error {
	if d.closed {
		return ErrStreamClosed
	}
	if d.initErr != nil {
		return d.initErr
	}
	if d.done {
		return io.EOF
	}

	var dataLines []string
	totalBytes := 0
	for d.scanner.Scan() {
		if len(d.scanner.Bytes()) > DefaultMaxRecordBytes {
			d.lastErr = fmt.Errorf("%w: limit is %d bytes", ErrRecordTooLarge, DefaultMaxRecordBytes)
			return d.lastErr
		}
		line := d.scanner.Text()

		// 空行用于提交当前 SSE 事件。
		if line == "" {
			decoded, err := d.decodeSSEEvent(v, dataLines)
			if err != nil || decoded {
				return err
			}
			dataLines = nil
			totalBytes = 0
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}

		if line == "data" || strings.HasPrefix(line, "data:") {
			data := ""
			if line != "data" {
				data = strings.TrimPrefix(line, "data:")
			}
			data = strings.TrimPrefix(data, " ")
			totalBytes += len(data)
			if len(dataLines) > 0 {
				totalBytes++
			}
			if totalBytes > DefaultMaxRecordBytes {
				d.lastErr = fmt.Errorf("%w: limit is %d bytes", ErrRecordTooLarge, DefaultMaxRecordBytes)
				return d.lastErr
			}
			dataLines = append(dataLines, data)
		}
	}

	if err := d.scanner.Err(); err != nil {
		d.lastErr = classifyScannerError(err)
		return d.lastErr
	}

	if len(dataLines) > 0 {
		decoded, err := d.decodeSSEEvent(v, dataLines)
		d.done = true
		if err != nil || decoded {
			return err
		}
	}
	d.done = true
	return io.EOF
}

func (d *SSEJSONDecoder) decodeSSEEvent(v any, dataLines []string) (bool, error) {
	if len(dataLines) == 0 {
		return false, nil
	}
	data := strings.Join(dataLines, "\n")
	if data == d.doneToken {
		d.done = true
		return true, io.EOF
	}
	if strings.TrimSpace(data) == "" {
		return false, nil
	}
	if err := decodeJSONBytes([]byte(data), v); err != nil {
		d.lastErr = err
		return false, err
	}
	return true, nil
}

// More 是否还有更多数据
func (d *SSEJSONDecoder) More() bool {
	return !d.closed && !d.done && d.initErr == nil && d.lastErr == nil
}

// Close 标记解码器为已关闭
func (d *SSEJSONDecoder) Close() {
	d.closed = true
	d.done = true
}

// Err 返回最后一个错误
func (d *SSEJSONDecoder) Err() error {
	return d.lastErr
}

// ============== 便捷函数 ==============

// DecodeStream 从流中解码所有 JSON 对象
func DecodeStream[T any](r io.Reader) ([]T, error) {
	decoder := NewStreamDecoder(r)
	var results []T

	for {
		var item T
		if err := decoder.Decode(&item); err != nil {
			if err == io.EOF {
				break
			}
			return results, err
		}
		results = append(results, item)
		if len(results) > DefaultMaxCollectionItems {
			return results[:DefaultMaxCollectionItems], ErrTooManyItems
		}
	}

	return results, nil
}

// DecodeNDJSON 从 NDJSON 流中解码所有对象
func DecodeNDJSON[T any](r io.Reader) ([]T, error) {
	decoder := NewNDJSONDecoder(newBoundedReader(r, DefaultMaxDocumentBytes, ErrDocumentTooLarge))
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
		if len(results) > DefaultMaxCollectionItems {
			return results[:DefaultMaxCollectionItems], ErrTooManyItems
		}
	}

	return results, nil
}

// DecodeSSEJSON 从 SSE 流中解码所有 JSON 对象
func DecodeSSEJSON[T any](r io.Reader) ([]T, error) {
	decoder := NewSSEJSONDecoder(newBoundedReader(r, DefaultMaxDocumentBytes, ErrDocumentTooLarge))
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
		if len(results) > DefaultMaxCollectionItems {
			return results[:DefaultMaxCollectionItems], ErrTooManyItems
		}
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
	decoder *StreamDecoder
	current T
	err     error
	done    bool
}

// NewStreamIterator 创建流迭代器
func NewStreamIterator[T any](r io.Reader) *StreamIterator[T] {
	return &StreamIterator[T]{
		decoder: NewStreamDecoder(r),
	}
}

// Next 读取下一个元素
func (it *StreamIterator[T]) Next() bool {
	if it.done {
		return false
	}

	var item T
	if err := it.decoder.Decode(&item); err != nil {
		if err == io.EOF {
			it.done = true
			return false
		}
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

func newLineScanner(r io.Reader, maximum int) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	// Scanner 的内部缓冲区还需要容纳 CRLF 分隔符，记录本身的上限由调用方检查。
	scannerMaximum := maximum
	if maximum <= int(^uint(0)>>1)-2 {
		scannerMaximum += 2
	}
	initial := 64 << 10
	if scannerMaximum < initial {
		initial = scannerMaximum
	}
	scanner.Buffer(make([]byte, initial), scannerMaximum)
	return scanner
}

func classifyScannerError(err error) error {
	if errors.Is(err, bufio.ErrTooLong) {
		return fmt.Errorf("%w: %w", ErrRecordTooLarge, err)
	}
	return fmt.Errorf("scan JSON stream: %w", err)
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

type boundedReader struct {
	reader    io.Reader
	remaining int64
	limitErr  error
}

func newBoundedReader(r io.Reader, maximum int, limitErr error) io.Reader {
	if isNilInterface(r) {
		return nil
	}
	return &boundedReader{reader: r, remaining: int64(maximum), limitErr: limitErr}
}

func (r *boundedReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.remaining > 0 {
		if int64(len(p)) > r.remaining {
			p = p[:r.remaining]
		}
		n, err := r.reader.Read(p)
		r.remaining -= int64(n)
		return n, err
	}
	var probe [1]byte
	n, err := r.reader.Read(probe[:])
	if n > 0 {
		return 0, r.limitErr
	}
	return 0, err
}
