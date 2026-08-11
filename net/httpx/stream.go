package httpx

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

var (
	// ErrStreamClosed 流已关闭
	ErrStreamClosed = errors.New("stream closed")
	// ErrInvalidSSE 无效的 SSE 格式
	ErrInvalidSSE = errors.New("invalid SSE format")
	// ErrStreamEventTooLarge 表示单个流事件超过允许的内存上限。
	ErrStreamEventTooLarge = errors.New("stream event exceeds configured limit")
	// ErrStreamCollectionLimit 表示流式响应聚合结果超过配置的总量上限。
	ErrStreamCollectionLimit = errors.New("stream collection exceeds configured limit")
)

const (
	// DefaultMaxStreamEventSize 是单个流事件默认允许的最大字节数。
	DefaultMaxStreamEventSize = 1 << 20
	// DefaultMaxCollectionBytes 是聚合器默认允许收集的 SSE data 总字节数。
	DefaultMaxCollectionBytes int64 = 16 << 20
	// DefaultMaxCollectionEvents 是聚合器默认允许收集的事件总数。
	DefaultMaxCollectionEvents = 10_000
)

// StreamResponse 流式响应
type StreamResponse struct {
	StatusCode   int
	Status       string
	Headers      http.Header
	body         io.ReadCloser
	reader       *bufio.Reader
	readMu       sync.Mutex
	closeOnce    sync.Once
	closeErr     error
	closed       atomic.Bool
	maxEventSize int
}

// SSEEvent Server-Sent Event 事件
type SSEEvent struct {
	ID    string // 事件 ID
	Event string // 事件类型
	Data  string // 事件数据
	Retry int    // 重连时间（毫秒）
}

// StreamOption 流式请求配置
type StreamOption func(*streamConfig)

type streamConfig struct {
	bufferSize   int
	maxEventSize int
}

// CollectionOption 配置一次流聚合操作的内存边界。
type CollectionOption func(*collectionConfig) error

type collectionConfig struct {
	maxBytes  int64
	maxEvents int
}

type collectionTracker struct {
	config collectionConfig
	bytes  int64
	events int
}

// WithBufferSize 设置读取缓冲区大小
func WithBufferSize(size int) StreamOption {
	return func(c *streamConfig) {
		c.bufferSize = size
	}
}

// WithMaxEventSize 设置单个流事件允许的最大字节数。
func WithMaxEventSize(size int) StreamOption {
	return func(c *streamConfig) {
		c.maxEventSize = size
	}
}

// WithMaxCollectionBytes 设置聚合器允许收集的 SSE data 总字节数。
func WithMaxCollectionBytes(maxBytes int64) CollectionOption {
	return func(config *collectionConfig) error {
		if maxBytes <= 0 {
			return errors.New("maximum collection byte size must be positive")
		}
		config.maxBytes = maxBytes
		return nil
	}
}

// WithMaxCollectionEvents 设置聚合器允许收集的事件总数。
func WithMaxCollectionEvents(maxEvents int) CollectionOption {
	return func(config *collectionConfig) error {
		if maxEvents <= 0 {
			return errors.New("maximum collection event count must be positive")
		}
		config.maxEvents = maxEvents
		return nil
	}
}

func newCollectionTracker(options []CollectionOption) (*collectionTracker, error) {
	config := collectionConfig{
		maxBytes:  DefaultMaxCollectionBytes,
		maxEvents: DefaultMaxCollectionEvents,
	}
	for index, option := range options {
		if option == nil {
			return nil, fmt.Errorf("%w: collection option %d must not be nil", ErrInvalidClientConfig, index)
		}
		if err := option(&config); err != nil {
			return nil, fmt.Errorf("%w: collection option %d: %w", ErrInvalidClientConfig, index, err)
		}
	}
	return &collectionTracker{config: config}, nil
}

func (tracker *collectionTracker) add(data string) error {
	if tracker.events >= tracker.config.maxEvents {
		return fmt.Errorf(
			"%w: maximum event count is %d",
			ErrStreamCollectionLimit,
			tracker.config.maxEvents,
		)
	}
	dataSize := int64(len(data))
	if dataSize > tracker.config.maxBytes-tracker.bytes {
		return fmt.Errorf(
			"%w: maximum byte size is %d",
			ErrStreamCollectionLimit,
			tracker.config.maxBytes,
		)
	}
	tracker.events++
	tracker.bytes += dataSize
	return nil
}

// GetStream 发送流式 GET 请求
func (r *Request) GetStream(url string, opts ...StreamOption) (*StreamResponse, error) {
	r.method = http.MethodGet
	r.url = url
	return r.executeStream(opts...)
}

// PostStream 发送流式 POST 请求
func (r *Request) PostStream(url string, opts ...StreamOption) (*StreamResponse, error) {
	r.method = http.MethodPost
	r.url = url
	return r.executeStream(opts...)
}

// executeStream 执行流式请求
func (r *Request) executeStream(opts ...StreamOption) (*StreamResponse, error) {
	if r == nil || r.client == nil || r.client.client == nil {
		return nil, fmt.Errorf("%w: request client is missing", ErrInvalidRequest)
	}
	if r.ctx == nil {
		return nil, ErrInvalidContext
	}
	if r.method == "" {
		return nil, fmt.Errorf("%w: method is missing", ErrInvalidRequest)
	}
	if r.url == "" && r.client.baseURL == "" {
		return nil, fmt.Errorf("%w: URL is missing", ErrInvalidRequest)
	}
	if r.jsonErr != nil {
		return nil, r.jsonErr
	}

	cfg := &streamConfig{
		bufferSize:   4096,
		maxEventSize: DefaultMaxStreamEventSize,
	}
	for index, opt := range opts {
		if opt == nil {
			return nil, fmt.Errorf("%w: stream option %d must not be nil", ErrInvalidClientConfig, index)
		}
		opt(cfg)
		if cfg.bufferSize <= 0 {
			return nil, fmt.Errorf("%w: stream option %d: buffer size must be positive", ErrInvalidClientConfig, index)
		}
		if cfg.maxEventSize <= 0 {
			return nil, fmt.Errorf("%w: stream option %d: maximum event size must be positive", ErrInvalidClientConfig, index)
		}
	}

	fullURL, err := resolveRequestURL(r.client.baseURL, r.url, r.query)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(r.ctx, r.method, fullURL, r.body)
	if err != nil {
		return nil, err
	}

	// 设置默认请求头
	for k, v := range r.client.headers {
		req.Header.Set(k, v)
	}

	// 设置请求特定的请求头
	for k, v := range r.headers {
		req.Header.Set(k, v)
	}

	// 设置 Accept 头以接收 SSE
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "text/event-stream")
	}

	httpResp, err := r.client.client.Do(req) //nolint:bodyclose // StreamResponse 接管响应体。
	if err != nil {
		return nil, err
	}

	return &StreamResponse{
		StatusCode:   httpResp.StatusCode,
		Status:       httpResp.Status,
		Headers:      httpResp.Header,
		body:         httpResp.Body,
		reader:       bufio.NewReaderSize(httpResp.Body, cfg.bufferSize),
		maxEventSize: cfg.maxEventSize,
	}, nil
}

// ReadLine 读取一行数据
func (s *StreamResponse) ReadLine() (string, error) {
	s.readMu.Lock()
	defer s.readMu.Unlock()

	if s.closed.Load() {
		return "", ErrStreamClosed
	}

	return s.readLineLimited()
}

// ReadSSE 读取下一个 SSE 事件
func (s *StreamResponse) ReadSSE() (*SSEEvent, error) {
	s.readMu.Lock()
	defer s.readMu.Unlock()

	if s.closed.Load() {
		return nil, ErrStreamClosed
	}

	return s.readSSELocked()
}

// readSSELocked 内部方法：读取下一个 SSE 事件，调用者必须持有 mu 锁
func (s *StreamResponse) readSSELocked() (*SSEEvent, error) {
	event := &SSEEvent{}
	var dataLines []string
	eventSize := 0

	for {
		line, err := s.readLineLimited()
		atEOF := errors.Is(err, io.EOF)
		if err != nil && !atEOF {
			return nil, err
		}
		if len(line) > s.eventSizeLimit()-eventSize {
			return nil, s.eventLimitError()
		}
		eventSize += len(line)

		// 空行表示事件结束
		if line == "" {
			if event.Event != "" || len(dataLines) > 0 || event.ID != "" {
				event.Data = strings.Join(dataLines, "\n")
				return event, nil
			}
			if atEOF {
				return nil, io.EOF
			}
			continue
		}

		// 注释行按 SSE 规范忽略。
		if !strings.HasPrefix(line, ":") {
			field, value, hasColon := strings.Cut(line, ":")
			if !hasColon {
				value = ""
			}
			value = strings.TrimPrefix(value, " ")
			switch field {
			case "data":
				dataLines = append(dataLines, value)
			case "event":
				event.Event = value
			case "id":
				if !strings.ContainsRune(value, '\x00') {
					event.ID = value
				}
			case "retry":
				// 非十进制或溢出的 retry 值按 SSE 规范忽略。
				if retry, parseErr := parseRetry(value); parseErr == nil {
					event.Retry = retry
				}
			}
		}
		if atEOF {
			if event.Event != "" || len(dataLines) > 0 || event.ID != "" {
				event.Data = strings.Join(dataLines, "\n")
				return event, nil
			}
			return nil, io.EOF
		}
	}
}

func (s *StreamResponse) readLineLimited() (string, error) {
	limit := s.eventSizeLimit()
	buffer := make([]byte, 0, min(s.reader.Size(), limit))
	for {
		fragment, err := s.reader.ReadSlice('\n')
		if len(fragment) > limit-len(buffer) {
			return "", s.eventLimitError()
		}
		buffer = append(buffer, fragment...)
		switch {
		case err == nil:
			return strings.TrimRight(string(buffer), "\r\n"), nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF):
			return strings.TrimRight(string(buffer), "\r\n"), io.EOF
		default:
			return "", err
		}
	}
}

func (s *StreamResponse) eventSizeLimit() int {
	if s.maxEventSize > 0 {
		return s.maxEventSize
	}
	return DefaultMaxStreamEventSize
}

func (s *StreamResponse) eventLimitError() error {
	return errors.Join(ErrInvalidSSE, ErrStreamEventTooLarge, s.Close())
}

// ReadJSON 读取下一个 JSON 数据（从 SSE data 字段）
func (s *StreamResponse) ReadJSON(v any) error {
	s.readMu.Lock()
	defer s.readMu.Unlock()

	if s.closed.Load() {
		return ErrStreamClosed
	}

	event, err := s.readSSELocked()
	if err != nil {
		return err
	}

	// 跳过 [DONE] 标记（OpenAI 格式）
	if event.Data == "[DONE]" {
		return io.EOF
	}

	return json.Unmarshal([]byte(event.Data), v)
}

// ReadBytes 读取原始字节流
func (s *StreamResponse) ReadBytes(p []byte) (int, error) {
	s.readMu.Lock()
	defer s.readMu.Unlock()

	if s.closed.Load() {
		return 0, ErrStreamClosed
	}

	return s.reader.Read(p)
}

// Close 关闭流（并发安全）
func (s *StreamResponse) Close() error {
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		s.closeErr = s.body.Close()
	})
	return s.closeErr
}

// IsSuccess 判断是否成功
func (s *StreamResponse) IsSuccess() bool {
	return s.StatusCode >= 200 && s.StatusCode < 300
}

// IsError 判断是否错误
func (s *StreamResponse) IsError() bool {
	return s.StatusCode >= 400
}

// parseRetry 解析 retry 值
func parseRetry(s string) (int, error) {
	if s == "" {
		return 0, ErrInvalidSSE
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, ErrInvalidSSE
		}
	}
	retry, err := strconv.ParseUint(s, 10, strconv.IntSize)
	if err != nil {
		return 0, fmt.Errorf("%w: parse retry value: %w", ErrInvalidSSE, err)
	}
	return int(retry), nil
}

// ============== 流式迭代器 ==============

// SSEIterator SSE 事件迭代器
type SSEIterator struct {
	stream *StreamResponse
	err    error
}

// Events 返回 SSE 事件迭代器
func (s *StreamResponse) Events() *SSEIterator {
	return &SSEIterator{stream: s}
}

// Next 读取下一个事件，返回 false 表示结束
func (it *SSEIterator) Next() (*SSEEvent, bool) {
	event, err := it.stream.ReadSSE()
	if err != nil {
		if err != io.EOF {
			it.err = err
		}
		return nil, false
	}
	return event, true
}

// Err 返回迭代过程中的错误
func (it *SSEIterator) Err() error {
	return it.err
}

// ============== 便捷方法 ==============

// GetStream 发送流式 GET 请求
func GetStream(ctx context.Context, url string) (*StreamResponse, error) {
	return getDefaultClient().R(ctx).GetStream(url)
}

// PostStream 发送流式 POST 请求
func PostStream(ctx context.Context, url string, body any) (*StreamResponse, error) {
	return getDefaultClient().R(ctx).SetJSONBody(body).PostStream(url)
}

// ============== 流式数据处理 ==============

// StreamHandler 流式数据处理函数
type StreamHandler func(event *SSEEvent) error

// OnData 设置数据处理回调
func (s *StreamResponse) OnData(handler StreamHandler) (err error) {
	defer func() { err = errors.Join(err, s.Close()) }()
	if handler == nil {
		return fmt.Errorf("%w: stream handler must not be nil", ErrInvalidRequest)
	}

	for {
		event, err := s.ReadSSE()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		// 跳过 [DONE] 标记
		if event.Data == "[DONE]" {
			return nil
		}

		if err := handler(event); err != nil {
			return err
		}
	}
}

// CollectData 在总字节数和总事件数边界内收集全部 data 字段。
// 任一读取或聚合错误都会返回 nil，避免调用方误用不完整结果。
func (s *StreamResponse) CollectData(options ...CollectionOption) (data []string, err error) {
	defer func() { err = errors.Join(err, s.Close()) }()
	tracker, err := newCollectionTracker(options)
	if err != nil {
		return nil, err
	}

	for {
		event, err := s.ReadSSE()
		if err != nil {
			if err == io.EOF {
				return data, nil
			}
			return nil, err
		}

		if event.Data == "[DONE]" {
			return data, nil
		}
		if err := tracker.add(event.Data); err != nil {
			return nil, err
		}

		data = append(data, event.Data)
	}
}

// CollectJSON 在总字节数和总事件数边界内收集并解析全部 JSON data 字段。
// 任一读取、解析或聚合错误都会返回 nil，避免调用方误用不完整结果。
func (s *StreamResponse) CollectJSON(factory func() any, options ...CollectionOption) (results []any, err error) {
	defer func() { err = errors.Join(err, s.Close()) }()
	if factory == nil {
		return nil, fmt.Errorf("%w: JSON factory must not be nil", ErrInvalidRequest)
	}
	tracker, err := newCollectionTracker(options)
	if err != nil {
		return nil, err
	}

	for {
		event, readErr := s.ReadSSE()
		if readErr != nil {
			if readErr == io.EOF {
				return results, nil
			}
			return nil, readErr
		}
		if event.Data == "[DONE]" {
			return results, nil
		}
		if err := tracker.add(event.Data); err != nil {
			return nil, err
		}

		v := factory()
		if err := json.Unmarshal([]byte(event.Data), v); err != nil {
			return nil, err
		}

		results = append(results, v)
	}
}

// ============== OpenAI 流式响应处理 ==============

// OpenAIStreamChunk OpenAI 流式响应块
type OpenAIStreamChunk struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role    string `json:"role,omitempty"`
			Content string `json:"content,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices"`
}

// ReadOpenAIChunk 读取 OpenAI 格式的流式响应块
func (s *StreamResponse) ReadOpenAIChunk() (*OpenAIStreamChunk, error) {
	var chunk OpenAIStreamChunk
	err := s.ReadJSON(&chunk)
	if err != nil {
		return nil, err
	}
	return &chunk, nil
}

// CollectOpenAIContent 在总字节数和总事件数边界内收集 OpenAI 流式内容。
// 字节上限按原始 SSE data 计算；任一错误都不返回部分内容。
func (s *StreamResponse) CollectOpenAIContent(options ...CollectionOption) (content string, err error) {
	defer func() { err = errors.Join(err, s.Close()) }()
	tracker, err := newCollectionTracker(options)
	if err != nil {
		return "", err
	}

	var builder bytes.Buffer
	for {
		event, readErr := s.ReadSSE()
		if readErr != nil {
			if readErr == io.EOF {
				return builder.String(), nil
			}
			return "", readErr
		}
		if event.Data == "[DONE]" {
			return builder.String(), nil
		}
		if err := tracker.add(event.Data); err != nil {
			return "", err
		}

		var chunk OpenAIStreamChunk
		if err := json.Unmarshal([]byte(event.Data), &chunk); err != nil {
			return "", err
		}

		if len(chunk.Choices) > 0 {
			builder.WriteString(chunk.Choices[0].Delta.Content)
		}
	}
}
