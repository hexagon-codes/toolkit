package sse

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	// ErrStreamClosed 流已关闭
	ErrStreamClosed = errors.New("sse: stream closed")
	// ErrConnectionFailed 连接失败
	ErrConnectionFailed = errors.New("sse: connection failed")
	// ErrInvalidEvent 无效的事件格式
	ErrInvalidEvent = errors.New("sse: invalid event format")
)

// Event 表示一个 SSE 事件
type Event struct {
	ID    string // 事件 ID
	Event string // 事件类型
	Data  string // 事件数据
	Retry int    // 重连时间（毫秒）
}

// IsEmpty 检查事件是否为空
func (e *Event) IsEmpty() bool {
	return e.ID == "" && e.Event == "" && e.Data == ""
}

// JSON 将 Data 解析为 JSON
func (e *Event) JSON(v any) error {
	return json.Unmarshal([]byte(e.Data), v)
}

// ============== SSE Reader ==============

// Reader SSE 事件读取器
type Reader struct {
	reader  *bufio.Reader
	closed  bool
	lastID  string
	mu      sync.Mutex
}

// NewReader 创建 SSE 事件读取器
func NewReader(r io.Reader) *Reader {
	return &Reader{
		reader: bufio.NewReader(r),
	}
}

// NewReaderWithSize 创建指定缓冲区大小的 SSE 事件读取器
func NewReaderWithSize(r io.Reader, size int) *Reader {
	return &Reader{
		reader: bufio.NewReaderSize(r, size),
	}
}

// Read 读取下一个 SSE 事件
func (r *Reader) Read() (*Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil, ErrStreamClosed
	}

	event := &Event{}
	var dataLines []string

	for {
		line, err := r.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				// 处理最后一行（可能没有换行符）
				if line != "" {
					line = strings.TrimRight(line, "\r\n")
					if strings.HasPrefix(line, "data:") {
						data := strings.TrimPrefix(line, "data:")
						if len(data) > 0 && data[0] == ' ' {
							data = data[1:]
						}
						dataLines = append(dataLines, data)
					} else if strings.HasPrefix(line, "event:") {
						event.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
					} else if strings.HasPrefix(line, "id:") {
						event.ID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
					}
				}
				// EOF 但有数据，返回最后的事件
				if len(dataLines) > 0 || event.Event != "" || event.ID != "" {
					event.Data = strings.Join(dataLines, "\n")
					if event.ID != "" {
						r.lastID = event.ID
					}
					return event, nil
				}
			}
			return nil, err
		}

		line = strings.TrimRight(line, "\r\n")

		// 空行表示事件结束
		if line == "" {
			if len(dataLines) > 0 || event.Event != "" || event.ID != "" {
				event.Data = strings.Join(dataLines, "\n")
				if event.ID != "" {
					r.lastID = event.ID
				}
				return event, nil
			}
			continue
		}

		// 解析字段
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			if len(data) > 0 && data[0] == ' ' {
				data = data[1:]
			}
			dataLines = append(dataLines, data)
		} else if strings.HasPrefix(line, "event:") {
			event.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "id:") {
			event.ID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		} else if strings.HasPrefix(line, "retry:") {
			if retry, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "retry:"))); err == nil {
				event.Retry = retry
			}
		} else if strings.HasPrefix(line, ":") {
			// 注释行，忽略
			continue
		}
	}
}

// LastEventID 返回最后接收的事件 ID
func (r *Reader) LastEventID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastID
}

// Close 关闭读取器
func (r *Reader) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
}

// ============== SSE Client ==============

// ClientConfig 客户端配置
type ClientConfig struct {
	// Headers 请求头
	Headers map[string]string
	// Timeout 连接超时
	Timeout time.Duration
	// RetryInterval 重连间隔
	RetryInterval time.Duration
	// MaxRetries 最大重试次数（0 表示无限）
	MaxRetries int
	// HTTPClient 自定义 HTTP 客户端
	HTTPClient *http.Client
	// LastEventID 上次事件 ID（用于断点续传）
	LastEventID string
}

// Client SSE 客户端
type Client struct {
	url    string
	config ClientConfig
}

// NewClient 创建 SSE 客户端
func NewClient(url string, opts ...ClientOption) *Client {
	c := &Client{
		url: url,
		config: ClientConfig{
			Headers:       make(map[string]string),
			Timeout:       30 * time.Second,
			RetryInterval: 3 * time.Second,
			MaxRetries:    0,
		},
	}

	for _, opt := range opts {
		opt(&c.config)
	}

	if c.config.HTTPClient == nil {
		c.config.HTTPClient = &http.Client{
			Timeout: c.config.Timeout,
		}
	}

	return c
}

// ClientOption 客户端选项
type ClientOption func(*ClientConfig)

// WithHeaders 设置请求头
func WithHeaders(headers map[string]string) ClientOption {
	return func(c *ClientConfig) {
		for k, v := range headers {
			c.Headers[k] = v
		}
	}
}

// WithTimeout 设置超时
func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *ClientConfig) {
		c.Timeout = timeout
	}
}

// WithRetryInterval 设置重连间隔
func WithRetryInterval(interval time.Duration) ClientOption {
	return func(c *ClientConfig) {
		c.RetryInterval = interval
	}
}

// WithMaxRetries 设置最大重试次数
func WithMaxRetries(n int) ClientOption {
	return func(c *ClientConfig) {
		c.MaxRetries = n
	}
}

// WithHTTPClient 设置自定义 HTTP 客户端
func WithHTTPClient(client *http.Client) ClientOption {
	return func(c *ClientConfig) {
		c.HTTPClient = client
	}
}

// WithLastEventID 设置上次事件 ID
func WithLastEventID(id string) ClientOption {
	return func(c *ClientConfig) {
		c.LastEventID = id
	}
}

// Stream SSE 事件流
type Stream struct {
	reader   *Reader
	response *http.Response
	events   chan *Event
	errors   chan error
	done     chan struct{}
	closed   bool
	mu       sync.Mutex
}

// Connect 连接到 SSE 端点
func (c *Client) Connect(ctx context.Context) (*Stream, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return nil, err
	}

	// 设置 SSE 所需的请求头
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")

	// 设置自定义请求头
	for k, v := range c.config.Headers {
		req.Header.Set(k, v)
	}

	// 设置 Last-Event-ID
	if c.config.LastEventID != "" {
		req.Header.Set("Last-Event-ID", c.config.LastEventID)
	}

	resp, err := c.config.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, &HTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
		}
	}

	stream := &Stream{
		reader:   NewReader(resp.Body),
		response: resp,
		events:   make(chan *Event, 100),
		errors:   make(chan error, 1),
		done:     make(chan struct{}),
	}

	// 启动事件读取协程
	go stream.readLoop()

	return stream, nil
}

// HTTPError HTTP 错误
type HTTPError struct {
	StatusCode int
	Status     string
}

func (e *HTTPError) Error() string {
	return "sse: HTTP " + e.Status
}

// readLoop 事件读取循环
func (s *Stream) readLoop() {
	defer close(s.events)
	defer close(s.errors)

	for {
		select {
		case <-s.done:
			return
		default:
			event, err := s.reader.Read()
			if err != nil {
				if err != io.EOF {
					select {
					case s.errors <- err:
					default:
					}
				}
				return
			}

			if !event.IsEmpty() {
				select {
				case s.events <- event:
				case <-s.done:
					return
				}
			}
		}
	}
}

// Events 返回事件通道
func (s *Stream) Events() <-chan *Event {
	return s.events
}

// Errors 返回错误通道
func (s *Stream) Errors() <-chan error {
	return s.errors
}

// LastEventID 返回最后接收的事件 ID
func (s *Stream) LastEventID() string {
	return s.reader.LastEventID()
}

// Close 关闭流
func (s *Stream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	close(s.done)
	s.reader.Close()
	return s.response.Body.Close()
}

// ============== SSE Writer（服务器端）==============

// Writer SSE 事件写入器
type Writer struct {
	w       http.ResponseWriter
	flusher http.Flusher
	closed  bool
	mu      sync.Mutex
}

// NewWriter 创建 SSE 事件写入器
func NewWriter(w http.ResponseWriter) *Writer {
	flusher, _ := w.(http.Flusher)

	// 设置 SSE 响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Nginx 禁用缓冲

	return &Writer{
		w:       w,
		flusher: flusher,
	}
}

// Write 写入 SSE 事件
func (w *Writer) Write(event *Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return ErrStreamClosed
	}

	var buf bytes.Buffer

	if event.ID != "" {
		buf.WriteString("id: ")
		buf.WriteString(event.ID)
		buf.WriteByte('\n')
	}

	if event.Event != "" {
		buf.WriteString("event: ")
		buf.WriteString(event.Event)
		buf.WriteByte('\n')
	}

	if event.Data != "" {
		lines := strings.Split(event.Data, "\n")
		for _, line := range lines {
			buf.WriteString("data: ")
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
	}

	if event.Retry > 0 {
		buf.WriteString("retry: ")
		buf.WriteString(strconv.Itoa(event.Retry))
		buf.WriteByte('\n')
	}

	buf.WriteByte('\n')

	_, err := w.w.Write(buf.Bytes())
	if err != nil {
		return err
	}

	if w.flusher != nil {
		w.flusher.Flush()
	}

	return nil
}

// WriteData 写入数据事件
func (w *Writer) WriteData(data string) error {
	return w.Write(&Event{Data: data})
}

// WriteJSON 写入 JSON 数据事件
func (w *Writer) WriteJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return w.WriteData(string(data))
}

// WriteComment 写入注释（用于保持连接）
func (w *Writer) WriteComment(comment string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return ErrStreamClosed
	}

	_, err := w.w.Write([]byte(": " + comment + "\n"))
	if err != nil {
		return err
	}

	if w.flusher != nil {
		w.flusher.Flush()
	}

	return nil
}

// Flush 刷新缓冲区
func (w *Writer) Flush() {
	if w.flusher != nil {
		w.flusher.Flush()
	}
}

// Close 关闭写入器
func (w *Writer) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
}

// ============== 便捷函数 ==============

// ParseEvent 从字符串解析 SSE 事件
func ParseEvent(data string) (*Event, error) {
	reader := NewReader(strings.NewReader(data + "\n\n"))
	return reader.Read()
}

// FormatEvent 将事件格式化为 SSE 字符串
func FormatEvent(event *Event) string {
	var buf bytes.Buffer

	if event.ID != "" {
		buf.WriteString("id: ")
		buf.WriteString(event.ID)
		buf.WriteByte('\n')
	}

	if event.Event != "" {
		buf.WriteString("event: ")
		buf.WriteString(event.Event)
		buf.WriteByte('\n')
	}

	if event.Data != "" {
		lines := strings.Split(event.Data, "\n")
		for _, line := range lines {
			buf.WriteString("data: ")
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
	}

	if event.Retry > 0 {
		buf.WriteString("retry: ")
		buf.WriteString(strconv.Itoa(event.Retry))
		buf.WriteByte('\n')
	}

	buf.WriteByte('\n')
	return buf.String()
}

// ============== AI API 专用 ==============

// OpenAIDoneToken OpenAI 流式响应结束标记
const OpenAIDoneToken = "[DONE]"

// IsOpenAIDone 检查是否是 OpenAI 结束标记
func IsOpenAIDone(event *Event) bool {
	return strings.TrimSpace(event.Data) == OpenAIDoneToken
}

// ReadOpenAIStream 读取 OpenAI 格式的流式响应
func ReadOpenAIStream[T any](r io.Reader, handler func(T) error) error {
	reader := NewReader(r)

	for {
		event, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		if IsOpenAIDone(event) {
			return nil
		}

		var item T
		if err := event.JSON(&item); err != nil {
			return err
		}

		if err := handler(item); err != nil {
			return err
		}
	}
}

// CollectOpenAIStream 收集 OpenAI 格式的所有流式响应
func CollectOpenAIStream[T any](r io.Reader) ([]T, error) {
	var results []T
	err := ReadOpenAIStream(r, func(item T) error {
		results = append(results, item)
		return nil
	})
	return results, err
}
