package httpx

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

var (
	// ErrStreamClosed 流已关闭
	ErrStreamClosed = errors.New("stream closed")
	// ErrInvalidSSE 无效的 SSE 格式
	ErrInvalidSSE = errors.New("invalid SSE format")
)

// StreamResponse 流式响应
type StreamResponse struct {
	StatusCode int
	Status     string
	Headers    http.Header
	body       io.ReadCloser
	reader     *bufio.Reader
	closed     bool
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
	bufferSize int
}

// WithBufferSize 设置读取缓冲区大小
func WithBufferSize(size int) StreamOption {
	return func(c *streamConfig) {
		c.bufferSize = size
	}
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
	if r.jsonErr != nil {
		return nil, r.jsonErr
	}

	cfg := &streamConfig{
		bufferSize: 4096,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	fullURL := r.url
	if r.client.baseURL != "" && !strings.HasPrefix(r.url, "http") {
		fullURL = r.client.baseURL + "/" + strings.TrimLeft(r.url, "/")
	}

	if len(r.query) > 0 {
		if strings.Contains(fullURL, "?") {
			fullURL += "&" + r.query.Encode()
		} else {
			fullURL += "?" + r.query.Encode()
		}
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

	httpResp, err := r.client.client.Do(req)
	if err != nil {
		return nil, err
	}

	return &StreamResponse{
		StatusCode: httpResp.StatusCode,
		Status:     httpResp.Status,
		Headers:    httpResp.Header,
		body:       httpResp.Body,
		reader:     bufio.NewReaderSize(httpResp.Body, cfg.bufferSize),
	}, nil
}

// ReadLine 读取一行数据
func (s *StreamResponse) ReadLine() (string, error) {
	if s.closed {
		return "", ErrStreamClosed
	}

	line, err := s.reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	return strings.TrimRight(line, "\r\n"), nil
}

// ReadSSE 读取下一个 SSE 事件
func (s *StreamResponse) ReadSSE() (*SSEEvent, error) {
	if s.closed {
		return nil, ErrStreamClosed
	}

	event := &SSEEvent{}
	var dataLines []string

	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF && (event.Event != "" || len(dataLines) > 0) {
				// EOF 但有数据，返回最后的事件
				event.Data = strings.Join(dataLines, "\n")
				return event, nil
			}
			return nil, err
		}

		line = strings.TrimRight(line, "\r\n")

		// 空行表示事件结束
		if line == "" {
			if event.Event != "" || len(dataLines) > 0 || event.ID != "" {
				event.Data = strings.Join(dataLines, "\n")
				return event, nil
			}
			continue
		}

		// 解析 SSE 字段
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			data = strings.TrimPrefix(data, " ")
			dataLines = append(dataLines, data)
		} else if strings.HasPrefix(line, "event:") {
			event.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "id:") {
			event.ID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		} else if strings.HasPrefix(line, "retry:") {
			// 忽略 retry 解析错误
			_, _ = parseRetry(strings.TrimSpace(strings.TrimPrefix(line, "retry:")))
		} else if strings.HasPrefix(line, ":") {
			// 注释行，忽略
			continue
		}
	}
}

// ReadJSON 读取下一个 JSON 数据（从 SSE data 字段）
func (s *StreamResponse) ReadJSON(v any) error {
	event, err := s.ReadSSE()
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
	if s.closed {
		return 0, ErrStreamClosed
	}

	return s.reader.Read(p)
}

// Close 关闭流
func (s *StreamResponse) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.body.Close()
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
	var retry int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, ErrInvalidSSE
		}
		retry = retry*10 + int(c-'0')
	}
	return retry, nil
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
	return NewClient().R().SetContext(ctx).GetStream(url)
}

// PostStream 发送流式 POST 请求
func PostStream(ctx context.Context, url string, body any) (*StreamResponse, error) {
	return NewClient().R().SetContext(ctx).SetJSONBody(body).PostStream(url)
}

// ============== 流式数据处理 ==============

// StreamHandler 流式数据处理函数
type StreamHandler func(event *SSEEvent) error

// OnData 设置数据处理回调
func (s *StreamResponse) OnData(handler StreamHandler) error {
	defer s.Close()

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

// CollectData 收集所有数据
func (s *StreamResponse) CollectData() ([]string, error) {
	defer s.Close()

	var data []string
	for {
		event, err := s.ReadSSE()
		if err != nil {
			if err == io.EOF {
				return data, nil
			}
			return data, err
		}

		if event.Data == "[DONE]" {
			return data, nil
		}

		data = append(data, event.Data)
	}
}

// CollectJSON 收集所有 JSON 数据
func (s *StreamResponse) CollectJSON(factory func() any) ([]any, error) {
	defer s.Close()

	var results []any
	for {
		v := factory()
		err := s.ReadJSON(v)
		if err != nil {
			if err == io.EOF {
				return results, nil
			}
			return results, err
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

// CollectOpenAIContent 收集 OpenAI 流式响应的所有内容
func (s *StreamResponse) CollectOpenAIContent() (string, error) {
	defer s.Close()

	var content bytes.Buffer
	for {
		chunk, err := s.ReadOpenAIChunk()
		if err != nil {
			if err == io.EOF {
				return content.String(), nil
			}
			return content.String(), err
		}

		if len(chunk.Choices) > 0 {
			content.WriteString(chunk.Choices[0].Delta.Content)
		}
	}
}
