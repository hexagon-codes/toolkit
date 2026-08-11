package sse

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net"
	"net/http"
	"net/textproto"
	neturl "net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	// ErrStreamClosed 流已关闭
	ErrStreamClosed = errors.New("sse: stream closed")
	// ErrInvalidEvent 无效的事件格式
	ErrInvalidEvent = errors.New("sse: invalid event format")
	// ErrInvalidReaderConfig 表示读取器配置无效。
	ErrInvalidReaderConfig = errors.New("sse: invalid reader configuration")
	// ErrInvalidClientConfig 表示客户端配置无效。
	ErrInvalidClientConfig = errors.New("sse: invalid client configuration")
	// ErrInvalidWriter 表示响应写入器无效。
	ErrInvalidWriter = errors.New("sse: invalid response writer")
	// ErrInvalidContext 表示调用方传入了空上下文。
	ErrInvalidContext = errors.New("sse: context must not be nil")
	// ErrInvalidHandler 表示调用方传入了空处理函数。
	ErrInvalidHandler = errors.New("sse: handler must not be nil")
	// ErrUnexpectedContentType 表示响应不是 SSE 媒体类型。
	ErrUnexpectedContentType = errors.New("sse: unexpected response content type")
	// ErrMaxBytesExceeded 读取累计字节数超过配置的上限
	//
	// 该错误用于防御不可信上游通过超长 SSE 流量耗尽内存的拒绝服务（DoS）攻击。
	// 仅当通过 WithMaxTotalBytes 配置了非零上限且累计读取字节数超过该上限时返回。
	//
	// 错误文案采用描述性措辞（含 "exceeded maximum total bytes"），便于下游对错误
	// 信息做断言或日志归类；错误身份保持不变，调用方应始终通过 errors.Is 判定，
	// 而非比较错误字符串。
	ErrMaxBytesExceeded = errors.New("sse: exceeded maximum total bytes limit")
	// ErrMaxLineBytesExceeded 表示单行超过读取器上限。
	ErrMaxLineBytesExceeded = errors.New("sse: exceeded maximum line bytes limit")
	// ErrMaxEventBytesExceeded 表示单个事件超过读取器上限。
	ErrMaxEventBytesExceeded = errors.New("sse: exceeded maximum event bytes limit")
	// ErrInvalidCollectionConfig 表示聚合读取缺少明确的资源预算。
	ErrInvalidCollectionConfig = errors.New("sse: invalid collection configuration")
	// ErrMaxEventsExceeded 表示聚合事件数超过调用方设置的上限。
	ErrMaxEventsExceeded = errors.New("sse: exceeded maximum collected events limit")
)

const (
	defaultMaxLineBytes   int64 = 1 << 20
	defaultMaxEventBytes  int64 = 8 << 20
	defaultEventBuffer          = 16
	maxHTTPErrorBodyBytes       = 64 << 10
)

// Event 表示一个 SSE 事件
type Event struct {
	ID    string // 事件 ID
	Event string // 事件类型
	Data  string // 事件数据
	Retry int    // 重连时间（毫秒）

	idSet    bool
	retrySet bool
}

// IsEmpty 检查事件是否为空
func (e *Event) IsEmpty() bool {
	if e == nil {
		return true
	}
	return e.ID == "" && e.Event == "" && e.Data == "" && !e.idSet && !e.retrySet && e.Retry == 0
}

// JSON 将 Data 解析为 JSON
func (e *Event) JSON(v any) error {
	if e == nil {
		return fmt.Errorf("%w: event must not be nil", ErrInvalidEvent)
	}
	return json.Unmarshal([]byte(e.Data), v)
}

// ============== SSE Reader ==============

// Reader SSE 事件读取器
//
// 默认情况下 Reader 使用标准 data 字段解析，并保留长连接的无限总寿命：
//   - data 字段：识别任意以 "data:" 开头的行，并自动剥离紧随冒号后的一个可选空格。
//   - 总字节数不设上限，但单行默认限制为 1 MiB、单事件默认限制为 8 MiB，避免无界内存增长。
//
// 通过 NewReaderWithOptions 配合 ReaderOption 可调整以下行为：
//   - WithMaxTotalBytes：限制累计读取字节数，超限返回 ErrMaxBytesExceeded，
//     用于防御不可信上游的内存耗尽型 DoS 攻击。
//   - WithMaxLineBytes / WithMaxEventBytes：覆盖默认的单行与单事件上限。
//   - WithStrictDataPrefix：启用严格 data 前缀模式，仅识别精确的 "data: "
//     （单空格）前缀，避免接受非标准 data 字段。
//
// 线程安全：所有方法均通过内部互斥锁保护，可并发调用。
type Reader struct {
	reader           *bufio.Reader
	source           io.Closer
	closed           bool
	lastID           string
	reconnectionTime time.Duration
	retrySet         bool
	atStart          bool
	skipLeadingLF    bool
	pendingCRBytes   int64
	terminalErr      error
	readMu           sync.Mutex
	mu               sync.RWMutex
	closeOnce        sync.Once
	closeErr         error

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
	// maxLineBytes 限制单行原始字节数，0 表示不限制。
	maxLineBytes int64
	// maxEventBytes 限制两个事件边界之间的原始字节数，0 表示不限制。
	maxEventBytes int64
	// doneFunc 为可选的事件级流结束判定函数（provider 无关的 done 谓词）。
	//
	// 为 nil 时（默认）不做任何结束判定。配置后，
	// 仅 ReadUntilDone 与 Each 会在每个非空事件上调用该函数：返回 true 即视为
	// 流的逻辑结束（如 OpenAI 的 "[DONE]"、Claude 的 message_stop、Gemini 的
	// finishReason 非空），由消费方注入各 provider 的判定规则，无需在本包写死。
	// 该谓词不影响底层 Read 的语义，Read 仍按 SSE 协议读取下一个事件。
	doneFunc func(*Event) bool
}

// ReaderOption 用于配置 Reader 的可选行为。
//
// 选项通过 NewReaderWithOptions 应用；未提供选项时使用 NewReader 的安全默认值。
type ReaderOption func(*Reader) error

// WithMaxTotalBytes 设置 Reader 累计读取字节数的上限（单位：字节）。
//
// 当累计读取的原始字节数（含换行符）超过 limit 时，Read 将返回 ErrMaxBytesExceeded，
// 从而中止对超长流的继续读取。该能力用于防御不可信上游通过无限或超长 SSE
// 响应耗尽进程内存的拒绝服务（DoS）攻击。
//
// 参数 limit 为 0 时表示不限制；负数属于无效配置。上限按累计字节计算，
// 而非单事件或单行字节，因此可有效约束整个流的总体内存占用。
func WithMaxTotalBytes(limit int64) ReaderOption {
	return func(r *Reader) error {
		if limit < 0 {
			return errors.New("maximum total bytes must not be negative")
		}
		r.maxTotalBytes = limit
		return nil
	}
}

// WithMaxLineBytes 设置单行原始字节数上限，0 表示不限制。
func WithMaxLineBytes(limit int64) ReaderOption {
	return func(r *Reader) error {
		if limit < 0 {
			return errors.New("maximum line bytes must not be negative")
		}
		r.maxLineBytes = limit
		return nil
	}
}

// WithMaxEventBytes 设置单个事件原始字节数上限，0 表示不限制。
func WithMaxEventBytes(limit int64) ReaderOption {
	return func(r *Reader) error {
		if limit < 0 {
			return errors.New("maximum event bytes must not be negative")
		}
		r.maxEventBytes = limit
		return nil
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
	return func(r *Reader) error {
		r.strictData = true
		return nil
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
//	r, err := sse.NewReaderWithOptions(body, sse.WithDoneFunc(sse.IsOpenAIDone))
//	if err != nil {
//	    return err
//	}
//	_ = r.Each(func(ev *sse.Event) error {
//	    // 处理增量 chunk……
//	    return nil
//	})
func WithDoneFunc(fn func(*Event) bool) ReaderOption {
	return func(r *Reader) error {
		if fn == nil {
			return errors.New("done function must not be nil")
		}
		r.doneFunc = fn
		return nil
	}
}

// NewReader 校验输入并创建 SSE 事件读取器。
func NewReader(r io.Reader) (*Reader, error) {
	if isNil(r) {
		return nil, fmt.Errorf("%w: source must not be nil", ErrInvalidReaderConfig)
	}
	reader := &Reader{
		reader:        bufio.NewReader(r),
		atStart:       true,
		maxLineBytes:  defaultMaxLineBytes,
		maxEventBytes: defaultMaxEventBytes,
	}
	if source, ok := r.(io.Closer); ok {
		reader.source = source
	}
	return reader, nil
}

// MustNewReader 创建读取器，输入无效时触发 panic。
// 仅应用于编译期已知且经过测试的输入。
func MustNewReader(r io.Reader) *Reader {
	reader, err := NewReader(r)
	if err != nil {
		panic(err)
	}
	return reader
}

// NewReaderWithSize 校验输入并创建指定缓冲区大小的 SSE 事件读取器。
func NewReaderWithSize(r io.Reader, size int) (*Reader, error) {
	if isNil(r) {
		return nil, fmt.Errorf("%w: source must not be nil", ErrInvalidReaderConfig)
	}
	if size <= 0 {
		return nil, fmt.Errorf("%w: buffer size must be positive", ErrInvalidReaderConfig)
	}
	reader := &Reader{
		reader:        bufio.NewReaderSize(r, size),
		atStart:       true,
		maxLineBytes:  defaultMaxLineBytes,
		maxEventBytes: defaultMaxEventBytes,
	}
	if source, ok := r.(io.Closer); ok {
		reader.source = source
	}
	return reader, nil
}

// MustNewReaderWithSize 创建指定缓冲区大小的读取器，配置无效时触发 panic。
func MustNewReaderWithSize(r io.Reader, size int) *Reader {
	reader, err := NewReaderWithSize(r, size)
	if err != nil {
		panic(err)
	}
	return reader
}

// NewReaderWithOptions 创建可配置的 SSE 事件读取器。
//
// 在 NewReader 的基础上，允许通过 ReaderOption 注入可选的安全增强能力，
// 例如 WithMaxTotalBytes（总字节上限，防 DoS）与 WithStrictDataPrefix
// （严格 data 前缀模式）。未传入任何选项时，行为与 NewReader 完全一致。
//
// 各选项之间相互独立，可任意组合。例如：
//
//	r, err := sse.NewReaderWithOptions(body,
//	    sse.WithMaxTotalBytes(8<<20),   // 累计上限 8 MiB
//	    sse.WithStrictDataPrefix(),     // 仅识别精确的 data: 加单空格前缀
//	)
//	if err != nil {
//	    return err
//	}
func NewReaderWithOptions(r io.Reader, opts ...ReaderOption) (*Reader, error) {
	reader, err := NewReader(r)
	if err != nil {
		return nil, err
	}
	for index, opt := range opts {
		if opt == nil {
			return nil, fmt.Errorf("%w: option %d must not be nil", ErrInvalidReaderConfig, index)
		}
		if err := opt(reader); err != nil {
			return nil, fmt.Errorf("%w: option %d: %w", ErrInvalidReaderConfig, index, err)
		}
	}
	return reader, nil
}

// MustNewReaderWithOptions 创建可配置读取器，配置无效时触发 panic。
func MustNewReaderWithOptions(r io.Reader, opts ...ReaderOption) *Reader {
	reader, err := NewReaderWithOptions(r, opts...)
	if err != nil {
		panic(err)
	}
	return reader
}

// Read 读取下一个 SSE 事件
func (r *Reader) Read() (*Event, error) {
	if r == nil || r.reader == nil {
		return nil, fmt.Errorf("%w: reader is not initialized", ErrInvalidReaderConfig)
	}
	r.readMu.Lock()
	defer r.readMu.Unlock()

	if r.isClosed() {
		return nil, ErrStreamClosed
	}
	if r.terminalErr != nil {
		return nil, r.terminalErr
	}

	event := &Event{}
	var dataLines []string
	var eventBytes int64

	for {
		line, rawLineBytes, err := r.readLine()
		if r.isClosed() {
			if err != nil {
				return nil, errors.Join(ErrStreamClosed, err)
			}
			return nil, ErrStreamClosed
		}
		line = strings.ToValidUTF8(line, "\uFFFD")
		if r.atStart {
			line = strings.TrimPrefix(line, "\ufeff")
			r.atStart = false
		}
		if rawLineBytes > math.MaxInt64-eventBytes ||
			r.maxEventBytes > 0 && rawLineBytes > r.maxEventBytes-eventBytes {
			return nil, r.setTerminalError(ErrMaxEventBytesExceeded)
		}
		eventBytes += rawLineBytes
		if line != "" {
			r.parseField(event, &dataLines, line)
		}

		if err != nil {
			return nil, r.setTerminalError(err)
		}

		// 空行表示事件结束
		if line == "" {
			if eventReady(event, dataLines) {
				return r.finishEvent(event, dataLines), nil
			}
			event = &Event{}
			dataLines = nil
			eventBytes = 0
			continue
		}
	}
}

func (r *Reader) parseField(event *Event, dataLines *[]string, line string) {
	if data, ok := r.matchData(line); ok {
		*dataLines = append(*dataLines, data)
		return
	}
	if strings.HasPrefix(line, ":") {
		return
	}

	field, value, found := strings.Cut(line, ":")
	if !found {
		value = ""
	}
	value = strings.TrimPrefix(value, " ")

	switch field {
	case "event":
		event.Event = value
	case "id":
		if !strings.ContainsRune(value, '\x00') {
			r.mu.Lock()
			r.lastID = value
			r.mu.Unlock()
			event.ID = value
			event.idSet = true
		}
	case "retry":
		if retry, ok := parseRetry(value); ok {
			r.mu.Lock()
			r.reconnectionTime = time.Duration(retry) * time.Millisecond
			r.retrySet = true
			r.mu.Unlock()
			event.Retry = retry
			event.retrySet = true
		}
	}
}

func parseRetry(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, false
		}
	}
	retry, err := strconv.ParseUint(value, 10, 63)
	if err != nil || retry > uint64((1<<63-1)/int64(time.Millisecond)) || retry > uint64(^uint(0)>>1) {
		return 0, false
	}
	return int(retry), true
}

func eventReady(event *Event, dataLines []string) bool {
	return len(dataLines) > 0
}

func (r *Reader) finishEvent(event *Event, dataLines []string) *Event {
	event.Data = strings.Join(dataLines, "\n")
	r.mu.RLock()
	event.ID = r.lastID
	r.mu.RUnlock()
	if event.Event == "" {
		event.Event = "message"
	}
	return event
}

func (r *Reader) isClosed() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.closed
}

func (r *Reader) setTerminalError(err error) error {
	if r.terminalErr == nil {
		r.terminalErr = err
	}
	return r.terminalErr
}

// readLine 识别 CRLF、单独 LF 和单独 CR，并在追加前执行字节上限检查。
func (r *Reader) readLine() (text string, rawBytes int64, err error) {
	line := make([]byte, 0, 128)
	var lineBytes int64
	var firstByte byte
	hasFirstByte := false

	if r.skipLeadingLF {
		character, err := r.readByte()
		r.skipLeadingLF = false
		if err != nil {
			r.pendingCRBytes = 0
			return "", 0, err
		}
		if character == '\n' {
			rawBytes = 1
			if r.pendingCRBytes == math.MaxInt64 ||
				r.maxLineBytes > 0 && r.pendingCRBytes >= r.maxLineBytes {
				r.pendingCRBytes = 0
				return "", rawBytes, ErrMaxLineBytesExceeded
			}
		} else {
			firstByte = character
			hasFirstByte = true
		}
		r.pendingCRBytes = 0
	}

	for {
		var character byte
		var err error
		if hasFirstByte {
			character = firstByte
			hasFirstByte = false
		} else {
			character, err = r.readByte()
			if err != nil {
				return string(line), rawBytes, err
			}
		}

		if rawBytes == math.MaxInt64 || lineBytes == math.MaxInt64 {
			return "", rawBytes, ErrMaxLineBytesExceeded
		}
		rawBytes++
		lineBytes++
		if r.maxLineBytes > 0 && lineBytes > r.maxLineBytes {
			return "", rawBytes, ErrMaxLineBytesExceeded
		}
		switch character {
		case '\n':
			return string(line), rawBytes, nil
		case '\r':
			r.skipLeadingLF = true
			r.pendingCRBytes = lineBytes
			return string(line), rawBytes, nil
		default:
			line = append(line, character)
		}
	}
}

func (r *Reader) readByte() (byte, error) {
	character, err := r.reader.ReadByte()
	if err != nil {
		return 0, err
	}
	if r.totalBytes == math.MaxInt64 || r.maxTotalBytes > 0 && r.totalBytes >= r.maxTotalBytes {
		return 0, ErrMaxBytesExceeded
	}
	r.totalBytes++
	return character, nil
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
	r.mu.RLock()
	fn := r.doneFunc
	r.mu.RUnlock()
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
	if handler == nil {
		return ErrInvalidHandler
	}
	for {
		ev, done, err := r.ReadUntilDone()
		if err != nil {
			if isOnlyEOF(err) {
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

	// 宽松模式遵循规范：无冒号的 data 字段值为空；有冒号时剥离一个可选前导空格。
	if line == "data" {
		return "", true
	}
	if strings.HasPrefix(line, "data:") {
		data := strings.TrimPrefix(line, "data:")
		if data != "" && data[0] == ' ' {
			data = data[1:]
		}
		return data, true
	}
	return "", false
}

func isOnlyEOF(err error) bool {
	if err == nil {
		return false
	}
	if multiError, ok := err.(interface{ Unwrap() []error }); ok {
		found := false
		for _, nested := range multiError.Unwrap() {
			if nested == nil {
				continue
			}
			found = true
			if !isOnlyEOF(nested) {
				return false
			}
		}
		return found
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		nested := wrapped.Unwrap()
		if nested != nil {
			return isOnlyEOF(nested)
		}
	}
	return err == io.EOF
}

// LastEventID 返回最后接收的事件 ID
func (r *Reader) LastEventID() string {
	if r == nil {
		return ""
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastID
}

// ReconnectionTime 返回最近一次有效 retry 字段设置的重连间隔。
func (r *Reader) ReconnectionTime() (time.Duration, bool) {
	if r == nil {
		return 0, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.reconnectionTime, r.retrySet
}

// Close 关闭读取器，并在底层输入实现 io.Closer 时中断正在进行的读取。
func (r *Reader) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()

	r.closeOnce.Do(func() {
		if r.source != nil {
			r.closeErr = r.source.Close()
		}
	})
	return r.closeErr
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

// ============== SSE Client ==============

// ClientConfig 客户端配置
type ClientConfig struct {
	// Headers 请求头
	Headers map[string]string
	// Timeout 连接超时
	Timeout time.Duration
	// HTTPClient 自定义 HTTP 客户端
	HTTPClient *http.Client
	// LastEventID 上次事件 ID（用于断点续传）
	LastEventID string
	// ReaderOptions 配置响应流解析器的资源与协议策略。
	ReaderOptions []ReaderOption
}

// Client SSE 客户端
type Client struct {
	url    string
	config ClientConfig
}

// NewClient 校验端点与选项并创建 SSE 客户端。
func NewClient(url string, opts ...ClientOption) (*Client, error) {
	normalizedURL, err := normalizeEndpoint(url)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidClientConfig, err)
	}
	config := ClientConfig{
		Headers: make(map[string]string),
		Timeout: 30 * time.Second,
	}

	for index, opt := range opts {
		if opt == nil {
			return nil, fmt.Errorf("%w: option %d must not be nil", ErrInvalidClientConfig, index)
		}
		if err := opt(&config); err != nil {
			return nil, fmt.Errorf("%w: option %d: %w", ErrInvalidClientConfig, index, err)
		}
	}
	if err := validateClientConfig(config); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidClientConfig, err)
	}

	immutableConfig := config
	immutableConfig.Headers = cloneHeaders(config.Headers)
	immutableConfig.ReaderOptions = append([]ReaderOption(nil), config.ReaderOptions...)

	if immutableConfig.HTTPClient == nil {
		defaultTransport, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			return nil, fmt.Errorf("%w: default transport cannot be cloned", ErrInvalidClientConfig)
		}
		// SSE 是长连接流式响应，不设置整体请求超时；仅限制流建立阶段。
		transport := defaultTransport.Clone()
		transport.DialContext = (&net.Dialer{
			Timeout:   immutableConfig.Timeout,
			KeepAlive: 30 * time.Second,
		}).DialContext
		transport.TLSHandshakeTimeout = immutableConfig.Timeout
		transport.ResponseHeaderTimeout = immutableConfig.Timeout
		immutableConfig.HTTPClient = &http.Client{
			Timeout:   0,
			Transport: transport,
		}
	}

	return &Client{url: normalizedURL, config: immutableConfig}, nil
}

// MustNewClient 创建 SSE 客户端，配置无效时触发 panic。
// 仅应用于编译期已知且经过测试的静态配置。
func MustNewClient(url string, opts ...ClientOption) *Client {
	client, err := NewClient(url, opts...)
	if err != nil {
		panic(err)
	}
	return client
}

// ClientOption 客户端选项
type ClientOption func(*ClientConfig) error

// WithHeaders 设置请求头
func WithHeaders(headers map[string]string) ClientOption {
	return func(c *ClientConfig) error {
		for key, value := range headers {
			if err := validateHeader(key, value); err != nil {
				return err
			}
		}
		if c.Headers == nil {
			c.Headers = make(map[string]string, len(headers))
		}
		for k, v := range headers {
			c.Headers[k] = v
		}
		return nil
	}
}

func validateClientConfig(config ClientConfig) error {
	if config.Timeout < 0 {
		return errors.New("timeout must not be negative")
	}
	if err := validateSingleLine("last event ID", config.LastEventID); err != nil {
		return err
	}
	for key, value := range config.Headers {
		if err := validateHeader(key, value); err != nil {
			return err
		}
	}
	if err := validateReaderOptions(config.ReaderOptions); err != nil {
		return err
	}
	return nil
}

func validateReaderOptions(options []ReaderOption) error {
	probe := &Reader{}
	for index, option := range options {
		if option == nil {
			return fmt.Errorf("%w: reader option %d must not be nil", ErrInvalidReaderConfig, index)
		}
		if err := option(probe); err != nil {
			return fmt.Errorf("%w: reader option %d: %w", ErrInvalidReaderConfig, index, err)
		}
	}
	return nil
}

func cloneHeaders(headers map[string]string) map[string]string {
	cloned := make(map[string]string, len(headers))
	for key, value := range headers {
		cloned[key] = value
	}
	return cloned
}

// WithTimeout 设置超时
func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *ClientConfig) error {
		if timeout < 0 {
			return errors.New("timeout must not be negative")
		}
		c.Timeout = timeout
		return nil
	}
}

// WithHTTPClient 设置自定义 HTTP 客户端
func WithHTTPClient(client *http.Client) ClientOption {
	return func(c *ClientConfig) error {
		if client == nil {
			return errors.New("HTTP client must not be nil")
		}
		c.HTTPClient = client
		return nil
	}
}

// WithLastEventID 设置上次事件 ID
func WithLastEventID(id string) ClientOption {
	return func(c *ClientConfig) error {
		if err := validateSingleLine("last event ID", id); err != nil {
			return err
		}
		c.LastEventID = id
		return nil
	}
}

// WithReaderOptions 设置响应流 Reader 使用的协议与资源上限。
func WithReaderOptions(options ...ReaderOption) ClientOption {
	copied := append([]ReaderOption(nil), options...)
	return func(config *ClientConfig) error {
		config.ReaderOptions = append(config.ReaderOptions, copied...)
		return nil
	}
}

func normalizeEndpoint(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	parsed, err := neturl.Parse(rawURL)
	if err != nil || parsed.Host == "" ||
		(!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) {
		return "", errors.New("endpoint must be an absolute HTTP or HTTPS URL")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return "", errors.New("endpoint must not contain user information or a fragment")
	}
	return rawURL, nil
}

func validateHeader(key, value string) error {
	if key == "" || textproto.CanonicalMIMEHeaderKey(key) == "" {
		return errors.New("header name is invalid")
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if (character < ' ' && character != '\t') || character == 0x7f {
			return fmt.Errorf("header %q contains an invalid value", key)
		}
	}
	return nil
}

func validateSingleLine(field, value string) error {
	if strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("%s must not contain line breaks or null bytes", field)
	}
	return nil
}

// Stream SSE 事件流
type Stream struct {
	reader   *Reader
	events   chan *Event
	errors   chan error
	ctx      context.Context
	cancel   context.CancelCauseFunc
	finished chan struct{}
}

// Connect 连接到 SSE 端点
func (c *Client) Connect(ctx context.Context) (*Stream, error) {
	if c == nil || c.config.HTTPClient == nil {
		return nil, fmt.Errorf("%w: client is not initialized", ErrInvalidClientConfig)
	}
	if isNil(ctx) {
		return nil, ErrInvalidContext
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, http.NoBody)
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

	resp, err := c.config.HTTPClient.Do(req) //nolint:bodyclose // 成功路径把响应体所有权交给 Reader。
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("%w: HTTP client returned a nil response", ErrInvalidClientConfig)
	}
	if isNil(resp.Body) {
		return nil, fmt.Errorf("%w: response body must not be nil", ErrInvalidReaderConfig)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, newHTTPResponseError(resp)
	}
	contentType := resp.Header.Get("Content-Type")
	mediaType, _, parseErr := mime.ParseMediaType(contentType)
	if parseErr != nil || !strings.EqualFold(mediaType, "text/event-stream") {
		contentTypeErr := fmt.Errorf("%w: got %q", ErrUnexpectedContentType, contentType)
		return nil, errors.Join(contentTypeErr, closeResponseBody(resp.Body))
	}

	reader, err := NewReaderWithOptions(resp.Body, c.config.ReaderOptions...)
	if err != nil {
		return nil, errors.Join(err, closeResponseBody(resp.Body))
	}

	return startStream(ctx, reader), nil
}

func closeResponseBody(body io.ReadCloser) error {
	if isNil(body) {
		return nil
	}
	return body.Close()
}

// CloseIdleConnections 关闭客户端当前持有的空闲连接。
func (c *Client) CloseIdleConnections() {
	if c == nil || c.config.HTTPClient == nil {
		return
	}
	c.config.HTTPClient.CloseIdleConnections()
}

// HTTPError HTTP 错误
type HTTPError struct {
	StatusCode int
	Status     string
	// Body 保存受限长度的响应正文，便于定位上游拒绝原因。
	Body string
	// BodyTruncated 表示响应正文超过诊断上限。
	BodyTruncated bool
}

func (e *HTTPError) Error() string {
	if e == nil {
		return "sse: unexpected HTTP response"
	}
	status := e.Status
	if status == "" {
		status = strconv.Itoa(e.StatusCode)
	}
	return "sse: HTTP " + status
}

func newHTTPResponseError(response *http.Response) error {
	data, readErr := io.ReadAll(io.LimitReader(response.Body, maxHTTPErrorBodyBytes+1))
	truncated := len(data) > maxHTTPErrorBodyBytes
	if truncated {
		data = data[:maxHTTPErrorBodyBytes]
	}
	httpErr := &HTTPError{
		StatusCode:    response.StatusCode,
		Status:        response.Status,
		Body:          strings.ToValidUTF8(string(data), "\uFFFD"),
		BodyTruncated: truncated,
	}
	closeErr := closeResponseBody(response.Body)
	if readErr == nil && closeErr == nil {
		return httpErr
	}
	return errors.Join(httpErr, readErr, closeErr)
}

// readLoop 事件读取循环
func (s *Stream) readLoop() {
	var readErr error
	defer func() {
		if recovered := recover(); recovered != nil {
			readErr = errors.Join(readErr, errors.New("sse: internal error in read loop"))
		}

		closeErr := s.reader.Close()
		if terminalErr := s.terminalError(readErr, closeErr); terminalErr != nil {
			s.errors <- terminalErr
		}
		close(s.events)
		close(s.errors)
		close(s.finished)
	}()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		event, err := s.reader.Read()
		if err != nil {
			readErr = err
			return
		}

		select {
		case s.events <- event:
		case <-s.ctx.Done():
			return
		}
	}
}

func startStream(ctx context.Context, reader *Reader) *Stream {
	streamContext, cancel := context.WithCancelCause(ctx)
	stream := &Stream{
		reader:   reader,
		events:   make(chan *Event, defaultEventBuffer),
		errors:   make(chan error, 1),
		ctx:      streamContext,
		cancel:   cancel,
		finished: make(chan struct{}),
	}
	go stream.closeReaderOnCancellation()
	go stream.readLoop()
	return stream
}

// closeReaderOnCancellation 确保取消能够打断阻塞在底层响应体上的读取。
func (s *Stream) closeReaderOnCancellation() {
	select {
	case <-s.ctx.Done():
		_ = s.reader.Close() //nolint:errcheck // 关闭错误由读取循环统一汇总。
	case <-s.finished:
	}
}

func (s *Stream) terminalError(readErr, closeErr error) error {
	cause := context.Cause(s.ctx)
	if cause != nil {
		if errors.Is(readErr, ErrStreamClosed) {
			readErr = nil
		}
		if !errors.Is(cause, ErrStreamClosed) {
			readErr = errors.Join(cause, readErr)
		}
	} else if isOnlyEOF(readErr) {
		readErr = nil
	}
	return errors.Join(readErr, closeErr)
}

// Events 返回事件通道
func (s *Stream) Events() <-chan *Event {
	if s == nil {
		return nil
	}
	return s.events
}

// Errors 返回错误通道
func (s *Stream) Errors() <-chan error {
	if s == nil {
		return nil
	}
	return s.errors
}

// Done 返回在读取循环和资源释放全部结束后关闭的通道。
func (s *Stream) Done() <-chan struct{} {
	if s == nil {
		return nil
	}
	return s.finished
}

// LastEventID 返回最后接收的事件 ID
func (s *Stream) LastEventID() string {
	if s == nil || s.reader == nil {
		return ""
	}
	return s.reader.LastEventID()
}

// Close 关闭流
func (s *Stream) Close() error {
	if s == nil {
		return nil
	}
	if s.cancel != nil {
		s.cancel(ErrStreamClosed)
	}
	var closeErr error
	if s.reader != nil {
		closeErr = s.reader.Close()
	}
	if s.finished != nil {
		<-s.finished
	}
	return closeErr
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

// NewWriter 校验响应写入器并创建 SSE 事件写入器。
func NewWriter(w http.ResponseWriter) (*Writer, error) {
	if isNil(w) {
		return nil, ErrInvalidWriter
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		flusher = nil
	}

	// 设置 SSE 响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Nginx 禁用缓冲

	return &Writer{
		w:       w,
		flusher: flusher,
	}, nil
}

// MustNewWriter 创建 SSE 写入器，输入无效时触发 panic。
// 仅应用于 net/http 已保证 ResponseWriter 非空的处理链路。
func MustNewWriter(w http.ResponseWriter) *Writer {
	writer, err := NewWriter(w)
	if err != nil {
		panic(err)
	}
	return writer
}

// Write 写入 SSE 事件
func (w *Writer) Write(event *Event) error {
	if w == nil || isNil(w.w) {
		return ErrInvalidWriter
	}
	if err := validateEvent(event); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return ErrStreamClosed
	}

	// 复用缓冲区，避免每次分配
	w.buf.Reset()

	appendEvent(&w.buf, event)

	written, err := w.w.Write(w.buf.Bytes())
	if err != nil {
		return err
	}
	if written != w.buf.Len() {
		return io.ErrShortWrite
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
	if w == nil || isNil(w.w) {
		return ErrInvalidWriter
	}
	if err := validateSingleLine("comment", comment); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidEvent, err)
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return ErrStreamClosed
	}

	payload := []byte(": " + comment + "\n")
	written, err := w.w.Write(payload)
	if err != nil {
		return err
	}
	if written != len(payload) {
		return io.ErrShortWrite
	}

	if w.flusher != nil {
		w.flusher.Flush()
	}

	return nil
}

// Flush 刷新缓冲区
func (w *Writer) Flush() {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.flusher != nil {
		w.flusher.Flush()
	}
}

// Close 关闭写入器
func (w *Writer) Close() {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
}

// ============== 便捷函数 ==============

// ParseEvent 从字符串解析 SSE 事件
func ParseEvent(data string) (*Event, error) {
	reader, err := NewReader(strings.NewReader(data + "\n\n"))
	if err != nil {
		return nil, err
	}
	return reader.Read()
}

// FormatEvent 将事件格式化为 SSE 字符串
func FormatEvent(event *Event) (string, error) {
	if err := validateEvent(event); err != nil {
		return "", err
	}
	var buf bytes.Buffer
	appendEvent(&buf, event)
	return buf.String(), nil
}

func validateEvent(event *Event) error {
	if event == nil {
		return fmt.Errorf("%w: event must not be nil", ErrInvalidEvent)
	}
	if err := validateSingleLine("event ID", event.ID); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidEvent, err)
	}
	if err := validateSingleLine("event type", event.Event); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidEvent, err)
	}
	if event.Retry < 0 {
		return fmt.Errorf("%w: retry must not be negative", ErrInvalidEvent)
	}
	return nil
}

func appendEvent(buffer *bytes.Buffer, event *Event) {
	if event.ID != "" || event.idSet {
		buffer.WriteString("id: ")
		buffer.WriteString(event.ID)
		buffer.WriteByte('\n')
	}

	if event.Event != "" {
		buffer.WriteString("event: ")
		buffer.WriteString(event.Event)
		buffer.WriteByte('\n')
	}

	normalizedData := strings.ReplaceAll(event.Data, "\r\n", "\n")
	normalizedData = strings.ReplaceAll(normalizedData, "\r", "\n")
	for _, line := range strings.Split(normalizedData, "\n") {
		buffer.WriteString("data: ")
		buffer.WriteString(line)
		buffer.WriteByte('\n')
	}

	if event.Retry > 0 || event.retrySet {
		buffer.WriteString("retry: ")
		buffer.WriteString(strconv.Itoa(event.Retry))
		buffer.WriteByte('\n')
	}

	buffer.WriteByte('\n')
}

// ============== AI API 专用 ==============

// OpenAIDoneToken OpenAI 流式响应结束标记
const OpenAIDoneToken = "[DONE]"

// IsOpenAIDone 检查是否是 OpenAI 结束标记
func IsOpenAIDone(event *Event) bool {
	if event == nil {
		return false
	}
	return strings.TrimSpace(event.Data) == OpenAIDoneToken
}

// ReadOpenAIStream 读取 OpenAI 格式的流式响应。
//
// Reader 创建成功后，本函数接管实现 io.Closer 的输入，并在返回前恰好关闭一次。
// 关闭错误会与读取、解码或处理错误合并，调用方可通过 errors.Is/As 分别判定。
func ReadOpenAIStream[T any](r io.Reader, handler func(T) error) (err error) {
	reader, err := NewReader(r)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, reader.Close())
	}()
	if handler == nil {
		return ErrInvalidHandler
	}
	return consumeOpenAIStream(reader, 0, handler)
}

func consumeOpenAIStream[T any](reader *Reader, maxEvents int, handler func(T) error) error {
	consumed := 0
	for {
		event, err := reader.Read()
		if err != nil {
			if isOnlyEOF(err) {
				return nil
			}
			return err
		}

		if IsOpenAIDone(event) {
			return nil
		}
		if maxEvents > 0 && consumed >= maxEvents {
			return ErrMaxEventsExceeded
		}

		var item T
		if err := event.JSON(&item); err != nil {
			return err
		}

		if err := handler(item); err != nil {
			return err
		}
		consumed++
	}
}

// CollectConfig 定义聚合流必须遵守的显式资源预算。
type CollectConfig struct {
	// MaxEvents 限制最多保留的业务事件数，必须为正数。
	MaxEvents int
	// MaxTotalBytes 限制整个输入流读取的原始字节数，必须为正数。
	MaxTotalBytes int64
}

func (config CollectConfig) validate() error {
	if config.MaxEvents <= 0 {
		return fmt.Errorf("%w: maximum events must be positive", ErrInvalidCollectionConfig)
	}
	if config.MaxTotalBytes <= 0 {
		return fmt.Errorf("%w: maximum total bytes must be positive", ErrInvalidCollectionConfig)
	}
	return nil
}

// CollectOpenAIStream 在显式事件数与总字节预算内聚合 OpenAI 格式流。
//
// Reader 创建成功后，本函数接管实现 io.Closer 的输入，并在返回前恰好关闭一次。
// 任意读取、解码、预算或关闭错误都会返回 nil，避免把部分结果误认为完整聚合结果；
// 关闭错误会与原错误合并，调用方可通过 errors.Is/As 分别判定。
func CollectOpenAIStream[T any](r io.Reader, config CollectConfig) (results []T, err error) {
	reader, err := NewReader(r)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, reader.Close())
		if err != nil {
			results = nil
		}
	}()
	if err := config.validate(); err != nil {
		return nil, err
	}
	if err := WithMaxTotalBytes(config.MaxTotalBytes)(reader); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidCollectionConfig, err)
	}
	err = consumeOpenAIStream(reader, config.MaxEvents, func(item T) error {
		results = append(results, item)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}
