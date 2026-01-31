package sse

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReader_Read(t *testing.T) {
	input := `event: message
data: {"content": "Hello"}

event: message
data: {"content": " World"}

`
	reader := NewReader(strings.NewReader(input))

	// 第一个事件
	event1, err := reader.Read()
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
	event2, err := reader.Read()
	if err != nil {
		t.Fatalf("failed to read second event: %v", err)
	}
	if event2.Data != `{"content": " World"}` {
		t.Errorf("unexpected data: %s", event2.Data)
	}
}

func TestReader_MultilineData(t *testing.T) {
	input := `data: line1
data: line2
data: line3

`
	reader := NewReader(strings.NewReader(input))

	event, err := reader.Read()
	if err != nil {
		t.Fatalf("failed to read event: %v", err)
	}

	expected := "line1\nline2\nline3"
	if event.Data != expected {
		t.Errorf("expected '%s', got '%s'", expected, event.Data)
	}
}

func TestReader_EventWithID(t *testing.T) {
	input := `id: 123
event: update
data: hello

`
	reader := NewReader(strings.NewReader(input))

	event, err := reader.Read()
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

func TestReader_RetryField(t *testing.T) {
	input := `retry: 5000
data: test

`
	reader := NewReader(strings.NewReader(input))

	event, err := reader.Read()
	if err != nil {
		t.Fatalf("failed to read event: %v", err)
	}

	if event.Retry != 5000 {
		t.Errorf("expected retry 5000, got %d", event.Retry)
	}
}

func TestReader_Comment(t *testing.T) {
	input := `: this is a comment
data: actual data

`
	reader := NewReader(strings.NewReader(input))

	event, err := reader.Read()
	if err != nil {
		t.Fatalf("failed to read event: %v", err)
	}

	if event.Data != "actual data" {
		t.Errorf("expected 'actual data', got '%s'", event.Data)
	}
}

func TestReader_LastEventID(t *testing.T) {
	input := `id: event-1
data: first

id: event-2
data: second

`
	reader := NewReader(strings.NewReader(input))

	_, _ = reader.Read()
	if reader.LastEventID() != "event-1" {
		t.Errorf("expected last ID 'event-1', got '%s'", reader.LastEventID())
	}

	_, _ = reader.Read()
	if reader.LastEventID() != "event-2" {
		t.Errorf("expected last ID 'event-2', got '%s'", reader.LastEventID())
	}
}

func TestReader_Close(t *testing.T) {
	reader := NewReader(strings.NewReader(`data: test`))
	reader.Close()

	_, err := reader.Read()
	if err != ErrStreamClosed {
		t.Errorf("expected ErrStreamClosed, got %v", err)
	}
}

func TestEvent_JSON(t *testing.T) {
	event := &Event{Data: `{"id": 1, "name": "test"}`}

	var result struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	err := event.JSON(&result)
	if err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	if result.ID != 1 || result.Name != "test" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestEvent_IsEmpty(t *testing.T) {
	empty := &Event{}
	if !empty.IsEmpty() {
		t.Error("expected empty event")
	}

	notEmpty := &Event{Data: "test"}
	if notEmpty.IsEmpty() {
		t.Error("expected non-empty event")
	}
}

func TestWriter_Write(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := NewWriter(recorder)

	event := &Event{
		ID:    "123",
		Event: "message",
		Data:  "Hello World",
		Retry: 3000,
	}

	err := writer.Write(event)
	if err != nil {
		t.Fatalf("write error: %v", err)
	}

	expected := "id: 123\nevent: message\ndata: Hello World\nretry: 3000\n\n"
	if recorder.Body.String() != expected {
		t.Errorf("expected %q, got %q", expected, recorder.Body.String())
	}
}

func TestWriter_WriteMultilineData(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := NewWriter(recorder)

	event := &Event{Data: "line1\nline2\nline3"}

	err := writer.Write(event)
	if err != nil {
		t.Fatalf("write error: %v", err)
	}

	expected := "data: line1\ndata: line2\ndata: line3\n\n"
	if recorder.Body.String() != expected {
		t.Errorf("expected %q, got %q", expected, recorder.Body.String())
	}
}

func TestWriter_WriteData(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := NewWriter(recorder)

	err := writer.WriteData("simple message")
	if err != nil {
		t.Fatalf("write error: %v", err)
	}

	expected := "data: simple message\n\n"
	if recorder.Body.String() != expected {
		t.Errorf("expected %q, got %q", expected, recorder.Body.String())
	}
}

func TestWriter_WriteJSON(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := NewWriter(recorder)

	err := writer.WriteJSON(map[string]int{"id": 1})
	if err != nil {
		t.Fatalf("write error: %v", err)
	}

	expected := "data: {\"id\":1}\n\n"
	if recorder.Body.String() != expected {
		t.Errorf("expected %q, got %q", expected, recorder.Body.String())
	}
}

func TestWriter_WriteComment(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := NewWriter(recorder)

	err := writer.WriteComment("keep-alive")
	if err != nil {
		t.Fatalf("write error: %v", err)
	}

	expected := ": keep-alive\n"
	if recorder.Body.String() != expected {
		t.Errorf("expected %q, got %q", expected, recorder.Body.String())
	}
}

func TestWriter_Close(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := NewWriter(recorder)
	writer.Close()

	err := writer.Write(&Event{Data: "test"})
	if err != ErrStreamClosed {
		t.Errorf("expected ErrStreamClosed, got %v", err)
	}
}

func TestWriter_Headers(t *testing.T) {
	recorder := httptest.NewRecorder()
	_ = NewWriter(recorder)

	if recorder.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("unexpected Content-Type: %s", recorder.Header().Get("Content-Type"))
	}
	if recorder.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("unexpected Cache-Control: %s", recorder.Header().Get("Cache-Control"))
	}
}

func TestClient_Connect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("missing Accept header")
		}

		writer := NewWriter(w)
		writer.Write(&Event{Data: "hello"})
		writer.Write(&Event{Data: "world"})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("connect error: %v", err)
	}
	defer stream.Close()

	// 读取事件
	var events []*Event
	for event := range stream.Events() {
		events = append(events, event)
	}

	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

func TestClient_WithHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("missing Authorization header")
		}
		writer := NewWriter(w)
		writer.WriteData("ok")
	}))
	defer server.Close()

	client := NewClient(server.URL, WithHeaders(map[string]string{
		"Authorization": "Bearer token",
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("connect error: %v", err)
	}
	defer stream.Close()
}

func TestClient_WithLastEventID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Last-Event-ID") != "event-5" {
			t.Errorf("expected Last-Event-ID 'event-5', got '%s'", r.Header.Get("Last-Event-ID"))
		}
		writer := NewWriter(w)
		writer.WriteData("ok")
	}))
	defer server.Close()

	client := NewClient(server.URL, WithLastEventID("event-5"))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("connect error: %v", err)
	}
	defer stream.Close()
}

func TestClient_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.Connect(ctx)
	if err == nil {
		t.Fatal("expected error")
	}

	httpErr, ok := err.(*HTTPError)
	if !ok {
		t.Fatalf("expected *HTTPError, got %T", err)
	}
	if httpErr.StatusCode != 401 {
		t.Errorf("expected status 401, got %d", httpErr.StatusCode)
	}
}

func TestParseEvent(t *testing.T) {
	event, err := ParseEvent("event: test\ndata: hello")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if event.Event != "test" {
		t.Errorf("expected event 'test', got '%s'", event.Event)
	}
	if event.Data != "hello" {
		t.Errorf("expected data 'hello', got '%s'", event.Data)
	}
}

func TestFormatEvent(t *testing.T) {
	event := &Event{
		ID:    "1",
		Event: "message",
		Data:  "hello",
	}

	result := FormatEvent(event)
	expected := "id: 1\nevent: message\ndata: hello\n\n"

	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestIsOpenAIDone(t *testing.T) {
	doneEvent := &Event{Data: "[DONE]"}
	if !IsOpenAIDone(doneEvent) {
		t.Error("expected true for [DONE]")
	}

	normalEvent := &Event{Data: `{"content": "hello"}`}
	if IsOpenAIDone(normalEvent) {
		t.Error("expected false for normal event")
	}
}

func TestReadOpenAIStream(t *testing.T) {
	input := `data: {"id": 1}

data: {"id": 2}

data: [DONE]

`
	type Item struct {
		ID int `json:"id"`
	}

	var items []Item
	err := ReadOpenAIStream(strings.NewReader(input), func(item Item) error {
		items = append(items, item)
		return nil
	})
	if err != nil {
		t.Fatalf("read error: %v", err)
	}

	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestCollectOpenAIStream(t *testing.T) {
	input := `data: {"id": 1}

data: {"id": 2}

data: {"id": 3}

data: [DONE]

`
	type Item struct {
		ID int `json:"id"`
	}

	items, err := CollectOpenAIStream[Item](strings.NewReader(input))
	if err != nil {
		t.Fatalf("collect error: %v", err)
	}

	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
}

func TestNewReaderWithSize(t *testing.T) {
	input := `data: test

`
	reader := NewReaderWithSize(strings.NewReader(input), 1024)

	event, err := reader.Read()
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if event.Data != "test" {
		t.Errorf("expected 'test', got '%s'", event.Data)
	}
}

func TestStream_LastEventID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writer := NewWriter(w)
		writer.Write(&Event{ID: "evt-1", Data: "first"})
		writer.Write(&Event{ID: "evt-2", Data: "second"})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("connect error: %v", err)
	}
	defer stream.Close()

	// 读取所有事件
	for range stream.Events() {
	}

	lastID := stream.LastEventID()
	if lastID != "evt-2" {
		t.Errorf("expected 'evt-2', got '%s'", lastID)
	}
}

func TestReader_EOF(t *testing.T) {
	input := `data: last event`
	reader := NewReader(strings.NewReader(input))

	// 没有结束空行，应该在 EOF 时返回事件
	event, err := reader.Read()
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if event.Data != "last event" {
		t.Errorf("expected 'last event', got '%s'", event.Data)
	}

	// 再次读取应该返回 EOF
	_, err = reader.Read()
	if err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
}
