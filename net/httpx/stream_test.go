package httpx

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStreamResponse_ReadSSE(t *testing.T) {
	sseData := `event: message
data: {"content": "Hello"}

event: message
data: {"content": " World"}

data: [DONE]

`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseData))
	}))
	defer server.Close()

	client := NewClient()
	stream, err := client.R().SetContext(context.Background()).GetStream(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	// 第一个事件
	event1, err := stream.ReadSSE()
	if err != nil {
		t.Fatalf("failed to read first event: %v", err)
	}
	if event1.Event != "message" {
		t.Errorf("expected event 'message', got '%s'", event1.Event)
	}
	if event1.Data != `{"content": "Hello"}` {
		t.Errorf("unexpected data: %s", event1.Data)
	}

	// 第二个事件
	event2, err := stream.ReadSSE()
	if err != nil {
		t.Fatalf("failed to read second event: %v", err)
	}
	if event2.Data != `{"content": " World"}` {
		t.Errorf("unexpected data: %s", event2.Data)
	}

	// DONE 事件
	event3, err := stream.ReadSSE()
	if err != nil {
		t.Fatalf("failed to read DONE event: %v", err)
	}
	if event3.Data != "[DONE]" {
		t.Errorf("expected [DONE], got '%s'", event3.Data)
	}
}

func TestStreamResponse_ReadJSON(t *testing.T) {
	sseData := `data: {"id": 1, "name": "test"}

data: {"id": 2, "name": "test2"}

data: [DONE]

`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseData))
	}))
	defer server.Close()

	client := NewClient()
	stream, err := client.R().SetContext(context.Background()).GetStream(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	type Item struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	// 第一个 JSON
	var item1 Item
	err = stream.ReadJSON(&item1)
	if err != nil {
		t.Fatalf("failed to read first JSON: %v", err)
	}
	if item1.ID != 1 || item1.Name != "test" {
		t.Errorf("unexpected item: %+v", item1)
	}

	// 第二个 JSON
	var item2 Item
	err = stream.ReadJSON(&item2)
	if err != nil {
		t.Fatalf("failed to read second JSON: %v", err)
	}
	if item2.ID != 2 {
		t.Errorf("expected id 2, got %d", item2.ID)
	}

	// [DONE] 应该返回 EOF
	var item3 Item
	err = stream.ReadJSON(&item3)
	if err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
}

func TestStreamResponse_CollectData(t *testing.T) {
	sseData := `data: line1

data: line2

data: line3

data: [DONE]

`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseData))
	}))
	defer server.Close()

	client := NewClient()
	stream, err := client.R().SetContext(context.Background()).GetStream(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := stream.CollectData()
	if err != nil {
		t.Fatalf("failed to collect data: %v", err)
	}

	if len(data) != 3 {
		t.Errorf("expected 3 items, got %d", len(data))
	}
	if data[0] != "line1" || data[1] != "line2" || data[2] != "line3" {
		t.Errorf("unexpected data: %v", data)
	}
}

func TestStreamResponse_OnData(t *testing.T) {
	sseData := `data: chunk1

data: chunk2

data: [DONE]

`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseData))
	}))
	defer server.Close()

	client := NewClient()
	stream, err := client.R().SetContext(context.Background()).GetStream(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var collected []string
	err = stream.OnData(func(event *SSEEvent) error {
		collected = append(collected, event.Data)
		return nil
	})
	if err != nil {
		t.Fatalf("OnData error: %v", err)
	}

	if len(collected) != 2 {
		t.Errorf("expected 2 items, got %d", len(collected))
	}
}

func TestStreamResponse_PostStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json, got %s", r.Header.Get("Content-Type"))
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: response\n\n"))
	}))
	defer server.Close()

	client := NewClient()
	stream, err := client.R().
		SetContext(context.Background()).
		SetJSONBody(map[string]string{"msg": "hello"}).
		PostStream(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	event, err := stream.ReadSSE()
	if err != nil {
		t.Fatalf("failed to read event: %v", err)
	}
	if event.Data != "response" {
		t.Errorf("expected 'response', got '%s'", event.Data)
	}
}

func TestStreamResponse_Multiline(t *testing.T) {
	sseData := `data: line1
data: line2
data: line3

`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseData))
	}))
	defer server.Close()

	client := NewClient()
	stream, err := client.R().SetContext(context.Background()).GetStream(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	event, err := stream.ReadSSE()
	if err != nil {
		t.Fatalf("failed to read event: %v", err)
	}

	// 多行 data 应该用换行符连接
	expected := "line1\nline2\nline3"
	if event.Data != expected {
		t.Errorf("expected '%s', got '%s'", expected, event.Data)
	}
}

func TestStreamResponse_EventWithID(t *testing.T) {
	sseData := `id: 123
event: update
data: hello

`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseData))
	}))
	defer server.Close()

	client := NewClient()
	stream, err := client.R().SetContext(context.Background()).GetStream(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	event, err := stream.ReadSSE()
	if err != nil {
		t.Fatalf("failed to read event: %v", err)
	}

	if event.ID != "123" {
		t.Errorf("expected ID '123', got '%s'", event.ID)
	}
	if event.Event != "update" {
		t.Errorf("expected event 'update', got '%s'", event.Event)
	}
	if event.Data != "hello" {
		t.Errorf("expected data 'hello', got '%s'", event.Data)
	}
}

func TestStreamResponse_Comment(t *testing.T) {
	sseData := `: this is a comment
data: actual data

`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseData))
	}))
	defer server.Close()

	client := NewClient()
	stream, err := client.R().SetContext(context.Background()).GetStream(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	event, err := stream.ReadSSE()
	if err != nil {
		t.Fatalf("failed to read event: %v", err)
	}

	// 注释行应该被忽略
	if event.Data != "actual data" {
		t.Errorf("expected 'actual data', got '%s'", event.Data)
	}
}

func TestStreamResponse_IsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient()
	stream, err := client.R().SetContext(context.Background()).GetStream(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	if !stream.IsSuccess() {
		t.Error("expected IsSuccess to be true")
	}
	if stream.IsError() {
		t.Error("expected IsError to be false")
	}
}

func TestStreamResponse_Closed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: test\n\n"))
	}))
	defer server.Close()

	client := NewClient()
	stream, err := client.R().SetContext(context.Background()).GetStream(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stream.Close()

	// 关闭后读取应该返回 ErrStreamClosed
	_, err = stream.ReadSSE()
	if err != ErrStreamClosed {
		t.Errorf("expected ErrStreamClosed, got %v", err)
	}
}

func TestSSEIterator(t *testing.T) {
	sseData := `data: item1

data: item2

data: item3

`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseData))
	}))
	defer server.Close()

	client := NewClient()
	stream, err := client.R().SetContext(context.Background()).GetStream(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	var items []string
	iter := stream.Events()
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		items = append(items, event.Data)
	}

	if iter.Err() != nil {
		t.Errorf("unexpected error: %v", iter.Err())
	}

	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
}

func TestOpenAIStreamChunk(t *testing.T) {
	sseData := `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"content":" World"},"finish_reason":null}]}

data: [DONE]

`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseData))
	}))
	defer server.Close()

	client := NewClient()
	stream, err := client.R().SetContext(context.Background()).GetStream(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := stream.CollectOpenAIContent()
	if err != nil {
		t.Fatalf("failed to collect content: %v", err)
	}

	if content != "Hello World" {
		t.Errorf("expected 'Hello World', got '%s'", content)
	}
}

func TestGetStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: test\n\n"))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := GetStream(ctx, server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	event, err := stream.ReadSSE()
	if err != nil {
		t.Fatalf("failed to read event: %v", err)
	}
	if event.Data != "test" {
		t.Errorf("expected 'test', got '%s'", event.Data)
	}
}

func TestPostStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: response\n\n"))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := PostStream(ctx, server.URL, map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	event, err := stream.ReadSSE()
	if err != nil {
		t.Fatalf("failed to read event: %v", err)
	}
	if event.Data != "response" {
		t.Errorf("expected 'response', got '%s'", event.Data)
	}
}

func TestWithBufferSize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// 发送大量数据
		data := strings.Repeat("x", 1024)
		w.Write([]byte("data: " + data + "\n\n"))
	}))
	defer server.Close()

	client := NewClient()
	stream, err := client.R().
		SetContext(context.Background()).
		GetStream(server.URL, WithBufferSize(8192))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	event, err := stream.ReadSSE()
	if err != nil {
		t.Fatalf("failed to read event: %v", err)
	}
	if len(event.Data) != 1024 {
		t.Errorf("expected 1024 bytes, got %d", len(event.Data))
	}
}
