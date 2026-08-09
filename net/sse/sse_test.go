package sse

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestReader_Read(t *testing.T) {
	input := `event: message
data: {"content": "Hello"}

event: message
data: {"content": " World"}

`
	reader := MustNewReader(strings.NewReader(input))

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

type boundedCountingReader struct {
	remaining int
	read      int
}

func (r *boundedCountingReader) Read(buffer []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	count := min(len(buffer), r.remaining)
	for index := range count {
		buffer[index] = 'x'
	}
	r.remaining -= count
	r.read += count
	return count, nil
}

func TestReaderMaxTotalBytesStopsReadingAnOversizedLineEarly(t *testing.T) {
	source := &boundedCountingReader{remaining: 1 << 20}
	reader := MustNewReaderWithOptions(source, WithMaxTotalBytes(32))
	if _, err := reader.Read(); !errors.Is(err, ErrMaxBytesExceeded) {
		t.Fatalf("Read() error = %v, want ErrMaxBytesExceeded", err)
	}
	if source.read > 8<<10 {
		t.Fatalf("Read() consumed %d bytes before enforcing a 32-byte limit", source.read)
	}
}

type blockingReadCloser struct {
	started   chan struct{}
	closed    chan struct{}
	startOnce sync.Once
	closeOnce sync.Once
}

func newBlockingReadCloser() *blockingReadCloser {
	return &blockingReadCloser{started: make(chan struct{}), closed: make(chan struct{})}
}

func (r *blockingReadCloser) Read([]byte) (int, error) {
	r.startOnce.Do(func() { close(r.started) })
	<-r.closed
	return 0, io.ErrClosedPipe
}

func (r *blockingReadCloser) Close() error {
	r.closeOnce.Do(func() { close(r.closed) })
	return nil
}

func TestStreamCloseInterruptsBlockedRead(t *testing.T) {
	body := newBlockingReadCloser()
	stream := startStream(context.Background(), MustNewReader(body))
	select {
	case <-body.started:
	case <-time.After(time.Second):
		t.Fatal("stream did not begin reading")
	}

	closeResult := make(chan error, 1)
	go func() { closeResult <- stream.Close() }()
	select {
	case err := <-closeResult:
		if err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close() did not interrupt the blocked read")
	}
}

func TestStreamAppliesBackpressureWithoutDroppingEvents(t *testing.T) {
	const eventCount = 150
	var input strings.Builder
	for index := range eventCount {
		input.WriteString("data: ")
		input.WriteString(strconv.Itoa(index))
		input.WriteString("\n\n")
	}
	body := io.NopCloser(strings.NewReader(input.String()))
	stream := startStream(context.Background(), MustNewReader(body))
	time.Sleep(20 * time.Millisecond)

	var received []string
	for event := range stream.Events() {
		received = append(received, event.Data)
	}
	if len(received) != eventCount {
		t.Fatalf("received %d events, want %d", len(received), eventCount)
	}
	for index, value := range received {
		if value != strconv.Itoa(index) {
			t.Fatalf("event %d = %q", index, value)
		}
	}
}

func TestReader_MultilineData(t *testing.T) {
	input := `data: line1
data: line2
data: line3

`
	reader := MustNewReader(strings.NewReader(input))

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
	reader := MustNewReader(strings.NewReader(input))

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
	reader := MustNewReader(strings.NewReader(input))

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
	reader := MustNewReader(strings.NewReader(input))

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
	reader := MustNewReader(strings.NewReader(input))

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
	reader := MustNewReader(strings.NewReader(`data: test`))
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
	writer := MustNewWriter(recorder)

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
	writer := MustNewWriter(recorder)

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
	writer := MustNewWriter(recorder)

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
	writer := MustNewWriter(recorder)

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
	writer := MustNewWriter(recorder)

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
	writer := MustNewWriter(recorder)
	writer.Close()

	err := writer.Write(&Event{Data: "test"})
	if err != ErrStreamClosed {
		t.Errorf("expected ErrStreamClosed, got %v", err)
	}
}

func TestWriter_Headers(t *testing.T) {
	recorder := httptest.NewRecorder()
	_ = MustNewWriter(recorder)

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

		writer := MustNewWriter(w)
		writer.Write(&Event{Data: "hello"})
		writer.Write(&Event{Data: "world"})
	}))
	defer server.Close()

	client := MustNewClient(server.URL)
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
		writer := MustNewWriter(w)
		writer.WriteData("ok")
	}))
	defer server.Close()

	client := MustNewClient(server.URL, WithHeaders(map[string]string{
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
		writer := MustNewWriter(w)
		writer.WriteData("ok")
	}))
	defer server.Close()

	client := MustNewClient(server.URL, WithLastEventID("event-5"))

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

	client := MustNewClient(server.URL)
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

	result, err := FormatEvent(event)
	if err != nil {
		t.Fatalf("format error: %v", err)
	}
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
	reader := MustNewReaderWithSize(strings.NewReader(input), 1024)

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
		writer := MustNewWriter(w)
		writer.Write(&Event{ID: "evt-1", Data: "first"})
		writer.Write(&Event{ID: "evt-2", Data: "second"})
	}))
	defer server.Close()

	client := MustNewClient(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("connect error: %v", err)
	}
	defer stream.Close()

	// 读取所有事件
	for event := range stream.Events() {
		_ = event
	}

	lastID := stream.LastEventID()
	if lastID != "evt-2" {
		t.Errorf("expected 'evt-2', got '%s'", lastID)
	}
}

func TestReader_EOFDiscardsIncompleteEvent(t *testing.T) {
	input := "data: complete\n\ndata: incomplete"
	reader := MustNewReader(strings.NewReader(input))

	event, err := reader.Read()
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if event.Data != "complete" {
		t.Errorf("expected complete event, got %q", event.Data)
	}

	event, err = reader.Read()
	if event != nil || !errors.Is(err, io.EOF) {
		t.Fatalf("Read() = (%#v, %v), want discarded incomplete event and io.EOF", event, err)
	}
}
