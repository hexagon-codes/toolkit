package sse

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
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
	// ErrMaxBytesExceeded 读取累计字节数超过配置的上限
	//
	// 该错误用于防御不可信上游通过超长 SSE 流量耗尽内存的拒绝服务（DoS）攻击。
	// 仅当通过 WithMaxTotalBytes 配置了非零上限且累计读取字节数超过该上限时返回。
	//
	// 错误文案采用描述性措辞（含 "exceeded maximum total bytes"），便于下游对错误
	// 信息做断言或日志归类；错误身份保持不变，调用方应始终通过 errors.Is 判定，
	// 而非比较错误字符串。
	ErrMaxBytesExceeded = errors.New("sse: exceeded maximum total bytes limit")
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
//
// 默认情况下 Reader 行为宽松且无字节上限，与历史版本完全兼容：
//   - data 字段：识别任意以 "data:" 开头的行，并自动剥离紧随冒号后的一个可选空格。
//   - 无总字节上限：可无限读取，直到底层 io.Reader 返回 EOF 或错误。
//
// 通过 NewReaderWithOptions 配合 ReaderOption 可启用两类可选的安全增强能力：
//   - WithMaxTotalBytes：限制累计读取字节数，超限返回 ErrMaxBytesExceeded，
//     用于防御不可信上游的内存耗尽型 DoS 攻击。
//   - WithStrictDataPrefix：启用严格 data 前缀模式，仅识别精确的 "data:" 或
//     "data: "（单空格）前缀，避免将形如 "datax:" 之类的行误判为 data 字段。
//
// 线程安全：所有方法均通过内部互斥锁保护，可并发调用。
type Reader struct {
	reader *bufio.Reader
	closed bool
	lastID string
	mu     sync.Mutex

	// maxTotalBytes 为累计读取字节数的上限，单位为字节。
	// 值为 0 表示不限制（默认）。当累计读取字节数超过该上限时，
	// Read 返回 ErrMaxBytesExceeded。
	maxTotalBytes int64
	// totalBytes 记录自创建以来累计读取的原始字节数（含换行符）。
	// 该计数在 maxTotalBytes 为 0 时同样累加，但不会触发上限检查。
	totalBytes int64
	// strictData 为 true 时启用严格 data 前缀模式：
	// 仅识别精确的 "data:" 或 "data: " 前缀，不再宽松匹配任意 "data:" 开头的行。
	strictData bool
	// doneFunc 为可选的事件级流结束判定函数（provider 无关的 done 谓词）。
	//
	// 为 nil 时（默认）不做任何结束判定，Read 行为与历史版本完全一致。配置后，
	// 仅 ReadUntilDone 与 Each 会在每个非空事件上调用该函数：返回 true 即视为
	// 流的逻辑结束（如 OpenAI 的 "[DONE]"、Claude 的 message_stop、Gemini 的
	// finishReason 非空），由消费方注入各 provider 的判定规则，无需在本包写死。
	// 该谓词不影响底层 Read 的语义，Read 仍按 SSE 协议读取下一个事件。
	doneFunc func(*Event) bool
}

// ReaderOption 用于配置 Reader 的可选行为。
//
// 选项通过 NewReaderWithOptions 应用，未提供任何选项时 Reader 保持与
// NewReader 完全一致的默认行为，确保向后兼容。
type ReaderOption func(*Reader)

// WithMaxTotalBytes 设置 Reader 累计读取字节数的上限（单位：字节）。
//
// 当累计读取的原始字节数（含换行符）超过 max 时，Read 将返回 ErrMaxBytesExceeded，
// 从而中止对超长流的继续读取。该能力用于防御不可信上游通过无限或超长 SSE
// 响应耗尽进程内存的拒绝服务（DoS）攻击。
//
// 参数 max <= 0 表示不限制（与默认行为一致）。上限按累计字节计算，
// 而非单事件或单行字节，因此可有效约束整个流的总体内存占用。
func WithMaxTotalBytes(max int64) ReaderOption {
	return func(r *Reader) {
		if max < 0 {
			max = 0
		}
		r.maxTotalBytes = max
	}
}

// WithStrictDataPrefix 启用严格 data 前缀模式。
//
// 默认（宽松）模式遵循 WHATWG SSE 规范：任意以 "data:" 开头的行都会被视为
// data 字段，例如 "data:hello" 与 "data: hello" 均被接受，且会剥离冒号后紧随的
// 一个可选空格，二者结果均为 "hello"。
//
// 严格模式仅识别精确的 "data: "（data + 冒号 + 单个空格）前缀，data 值为该前缀
// 之后的全部内容（逐字保留，不再额外剥离空格）。不满足该前缀的行一律忽略，包括：
//   - "data:hello" —— 冒号后无空格，被忽略（严格模式下不视为 data 字段）；
//   - "data:"      —— 仅有前缀无空格，被忽略；
//   - "datax: v"   —— 字段名不为 data，被忽略。
//
// 该模式与部分上游（如 MCP over HTTP）实现保持一致：它们要求规范的 "data: "
// 形式以避免对非标准 data 行做出宽松解读，从而获得更确定、更安全的解析行为。
// 严格模式仅收紧 data 行的前缀判定，不改变多行 data 的拼接方式，也不改变
// event/id/retry/注释等其它字段的解析逻辑。
func WithStrictDataPrefix() ReaderOption {
	return func(r *Reader) {
		r.strictData = true
	}
}

// WithDoneFunc 设置可选的事件级流结束判定函数（provider 无关的 done 谓词）。
//
// 不同 AI 上游标记流结束的方式各异且互不兼容，例如：
//   - OpenAI：发送一个 data 值为 "[DONE]" 的事件；
//   - Anthropic Claude：发送 event 类型为 "message_stop" 的事件；
//   - Google Gemini：在 chunk 中携带非空的 finishReason 字段。
//
// 本包不应在内部写死任一上游的判定逻辑。WithDoneFunc 允许消费方注入自己的
// done 谓词：对每个非空事件回调 fn，当 fn 返回 true 时，ReadUntilDone 与 Each
// 视为流的逻辑结束并停止迭代。
//
// 该选项不改变 Read 的语义，也不影响 IsOpenAIDone 等既有便捷函数；fn 为 nil
// 时等价于未配置（不做结束判定）。fn 应当是无副作用的纯判定函数，且不得修改
// 传入的 *Event。
//
// 典型用法（以 OpenAI 为例，复用既有 IsOpenAIDone）：
//
//	r := sse.NewReaderWithOptions(body, sse.WithDoneFunc(sse.IsOpenAIDone))
//	_ = r.Each(func(ev *sse.Event) error {
//	    // 处理增量 chunk……
//	    return nil
//	})
func WithDoneFunc(fn func(*Event) bool) ReaderOption {
	return func(r *Reader) {
		r.doneFunc = fn
	}
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

// NewReaderWithOptions 创建可配置的 SSE 事件读取器。
//
// 在 NewReader 的基础上，允许通过 ReaderOption 注入可选的安全增强能力，
// 例如 WithMaxTotalBytes（总字节上限，防 DoS）与 WithStrictDataPrefix
// （严格 data 前缀模式）。未传入任何选项时，行为与 NewReader 完全一致。
//
// 各选项之间相互独立，可任意组合。例如：
//
//	r := sse.NewReaderWithOptions(body,
//	    sse.WithMaxTotalBytes(8<<20),   // 累计上限 8 MiB
//	    sse.WithStrictDataPrefix(),     // 仅识别精确 data: / data: 前缀
//	)
func NewReaderWithOptions(r io.Reader, opts ...ReaderOption) *Reader {
	reader := &Reader{
		reader: bufio.NewReader(r),
	}
	for _, opt := range opts {
		opt(reader)
	}
	return reader
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
		// 在剥离换行符之前累加原始字节数，确保上限按真实流量计算。
		// 即便本次读取以错误结束，已读到的部分字节也需计入累计值。
		if len(line) > 0 {
			r.totalBytes += int64(len(line))
			if r.maxTotalBytes > 0 && r.totalBytes > r.maxTotalBytes {
				return nil, ErrMaxBytesExceeded
			}
		}
		if err != nil {
			if err == io.EOF {
				// 处理最后一行（可能没有换行符）
				if line != "" {
					line = strings.TrimRight(line, "\r\n")
					if data, ok := r.matchData(line); ok {
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
		if data, ok := r.matchData(line); ok {
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

// ReadUntilDone 读取下一个事件，并在该事件命中 done 谓词时同时返回 done=true。
//
// 行为与 Read 完全对齐——按 SSE 协议读取下一个事件并返回 (*Event, error)——
// 额外返回一个布尔标记 done，用于告知调用方"本事件已被 done 谓词判定为流结束"。
//
// done 的取值规则：
//   - 未通过 WithDoneFunc 配置谓词（doneFunc 为 nil）时，done 恒为 false；
//   - 读取出错（含 io.EOF）时，done 为 false，event 可能为 nil，错误原样返回；
//   - 成功读取到事件后，对其调用 doneFunc，结果即为 done。
//
// 该方法不"吞掉"命中 done 的事件：命中时仍会把该事件随 done=true 一并返回，
// 由调用方决定是否处理（例如 OpenAI 的 "[DONE]" 哨兵事件通常应被忽略，而
// Gemini 末包仍携带有效增量需要处理）。线程安全，可与其它方法并发调用。
//
// 典型用法：
//
//	for {
//	    ev, done, err := r.ReadUntilDone()
//	    if err != nil { // 含 io.EOF
//	        break
//	    }
//	    handle(ev)
//	    if done {
//	        break
//	    }
//	}
func (r *Reader) ReadUntilDone() (event *Event, done bool, err error) {
	ev, err := r.Read()
	if err != nil {
		return ev, false, err
	}
	// 读锁内访问 doneFunc 字段，避免与潜在的并发配置产生数据竞争。
	// doneFunc 在构造期由 ReaderOption 设置、之后只读，这里加锁主要为与
	// Read/Close 等持锁方法保持一致的内存可见性语义。
	r.mu.Lock()
	fn := r.doneFunc
	r.mu.Unlock()
	if fn != nil {
		done = fn(ev)
	}
	return ev, done, nil
}

// Each 迭代读取事件并对每个非空事件调用 handler，直到流结束或 handler 返回错误。
//
// 迭代在以下任一情况下终止：
//   - 底层读取返回 io.EOF：视为正常结束，Each 返回 nil；
//   - 底层读取返回其它错误：Each 原样返回该错误；
//   - handler 返回非 nil 错误：Each 立即停止并返回该错误；
//   - 已通过 WithDoneFunc 配置 done 谓词，且某事件命中该谓词：Each 在
//     处理完该事件（仍会回调 handler）后正常结束并返回 nil。
//
// 命中 done 谓词的事件同样会传给 handler，由 handler 自行决定是否处理；
// 这与 ReadUntilDone 的"不吞事件"语义保持一致。未配置 done 谓词时，Each
// 等价于"读到 EOF/错误为止"的常规迭代。
//
// 该方法是对 ReadUntilDone 的便捷封装，适用于消费方只需顺序处理事件、
// 无需手写读取循环的场景。
func (r *Reader) Each(handler func(*Event) error) error {
	for {
		ev, done, err := r.ReadUntilDone()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if herr := handler(ev); herr != nil {
			return herr
		}
		if done {
			return nil
		}
	}
}

// matchData 判断给定行是否为 data 字段行，并返回剥离前缀后的 data 值。
//
// 返回值 ok 为 true 时，data 为提取出的数据内容；ok 为 false 时该行不是 data 行。
//
// 宽松模式（默认）：识别任意以 "data:" 开头的行，并剥离冒号后紧随的一个可选空格，
// 与历史行为完全一致。
//
// 严格模式（WithStrictDataPrefix）：仅识别精确的 "data: "（含单个空格）前缀，
// 返回该前缀之后的内容（逐字保留）；其余行（包括 "data:hello"、"data:" 等
// 不带空格的形式）一律不视为 data 字段。
func (r *Reader) matchData(line string) (string, bool) {
	if r.strictData {
		// 严格模式：仅接受规范的 "data: " 前缀（data + 冒号 + 单个空格），
		// 取其后全部内容作为 data 值，不再额外剥离空格。
		// "data:" 无空格、"datax:" 等形式均被判定为非 data 行而忽略。
		if strings.HasPrefix(line, "data: ") {
			return line[len("data: "):], true
		}
		return "", false
	}

	// 宽松模式：保持历史行为，识别任意 "data:" 开头并剥离一个可选前导空格。
	if strings.HasPrefix(line, "data:") {
		data := strings.TrimPrefix(line, "data:")
		if len(data) > 0 && data[0] == ' ' {
			data = data[1:]
		}
		return data, true
	}
	return "", false
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
		// SSE 是长连接流式响应，不应设置整体请求超时（http.Client.Timeout）
		// 连接超时通过 Transport 层控制
		c.config.HTTPClient = &http.Client{
			Timeout: 0,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   c.config.Timeout,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout: c.config.Timeout,
			},
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
	defer func() {
		// 保护 panic，确保资源被正确释放
		if r := recover(); r != nil {
			select {
			case s.errors <- errors.New("sse: internal error in readLoop"):
			default:
			}
		}
		close(s.events)
		close(s.errors)
	}()

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
				default:
					// 事件通道已满，丢弃最旧的事件以防止阻塞
					// 这是一个权衡：防止 goroutine 泄漏比丢失事件更重要
					select {
					case <-s.events: // 丢弃一个旧事件
						select {
						case s.events <- event: // 写入新事件
						case <-s.done:
							return
						default:
							// 如果还是失败，跳过这个事件
						}
					case <-s.done:
						return
					default:
						// 如果还是失败，跳过这个事件
					}
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
	buf     bytes.Buffer // 复用缓冲区，避免每次 Write 分配
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

	// 复用缓冲区，避免每次分配
	w.buf.Reset()

	if event.ID != "" {
		w.buf.WriteString("id: ")
		w.buf.WriteString(event.ID)
		w.buf.WriteByte('\n')
	}

	if event.Event != "" {
		w.buf.WriteString("event: ")
		w.buf.WriteString(event.Event)
		w.buf.WriteByte('\n')
	}

	if event.Data != "" {
		lines := strings.Split(event.Data, "\n")
		for _, line := range lines {
			w.buf.WriteString("data: ")
			w.buf.WriteString(line)
			w.buf.WriteByte('\n')
		}
	}

	if event.Retry > 0 {
		w.buf.WriteString("retry: ")
		w.buf.WriteString(strconv.Itoa(event.Retry))
		w.buf.WriteByte('\n')
	}

	w.buf.WriteByte('\n')

	_, err := w.w.Write(w.buf.Bytes())
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
