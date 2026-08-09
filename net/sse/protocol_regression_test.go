package sse

import (
	"bytes"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReaderDefaultLineLimitStopsUnboundedInput(t *testing.T) {
	source := &boundedCountingReader{remaining: 4 << 20}
	reader := MustNewReader(source)
	if _, err := reader.Read(); !errors.Is(err, ErrMaxLineBytesExceeded) {
		t.Fatalf("Read() error = %v, want ErrMaxLineBytesExceeded", err)
	}
	if source.read > 2<<20 {
		t.Fatalf("Read() consumed %d bytes before enforcing the default line limit", source.read)
	}
}

func TestReaderEventLimitBoundsManySmallLines(t *testing.T) {
	reader := MustNewReaderWithOptions(
		strings.NewReader("data: 1234\ndata: 5678\n\n"),
		WithMaxEventBytes(12),
	)
	if _, err := reader.Read(); !errors.Is(err, ErrMaxEventBytesExceeded) {
		t.Fatalf("Read() error = %v, want ErrMaxEventBytesExceeded", err)
	}
}

func TestReaderStoresRetryControlWithoutDispatchingEvent(t *testing.T) {
	reader := MustNewReader(strings.NewReader("retry: 1500\n\n"))
	event, err := reader.Read()
	if event != nil || !errors.Is(err, io.EOF) {
		t.Fatalf("Read() = (%#v, %v), want (nil, io.EOF)", event, err)
	}
	retry, ok := reader.ReconnectionTime()
	if !ok || retry != 1500*time.Millisecond {
		t.Fatalf("ReconnectionTime() = (%v, %v), want (1.5s, true)", retry, ok)
	}
}

func TestReaderIgnoresInvalidRetryAndNullID(t *testing.T) {
	reader := MustNewReader(strings.NewReader("id: bad\x00id\nretry: -1\ndata: ok\n\n"))
	event, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if event.ID != "" {
		t.Fatalf("Read() ID = %q, want empty", event.ID)
	}
	if event.Retry != 0 {
		t.Fatalf("Read() retry = %d, want 0", event.Retry)
	}
	if event.Data != "ok" {
		t.Fatalf("Read() data = %q, want ok", event.Data)
	}
	if reader.LastEventID() != "" {
		t.Fatalf("LastEventID() = %q, want empty", reader.LastEventID())
	}
}

func TestReaderPreservesSSEFieldWhitespace(t *testing.T) {
	reader := MustNewReader(strings.NewReader("event:  custom  \nid:  event-id  \ndata: ok\n\n"))
	event, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if event.Event != " custom  " {
		t.Fatalf("Read() event type = %q, want %q", event.Event, " custom  ")
	}
	if event.ID != " event-id  " {
		t.Fatalf("Read() ID = %q, want %q", event.ID, " event-id  ")
	}
}

func TestReaderRecognizesUTF8BOMAtStreamStart(t *testing.T) {
	reader := MustNewReader(strings.NewReader("\ufeffdata: ok\n\n"))
	event, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if event.Data != "ok" {
		t.Fatalf("Read() data = %q, want ok", event.Data)
	}
}

func TestReaderAcceptsAllSSELineEndings(t *testing.T) {
	input := "id: stream-1\revent: custom\r\ndata: first\ndata: second\r\rdata: third\r\r"
	reader := MustNewReader(strings.NewReader(input))

	first, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != "stream-1" || first.Event != "custom" || first.Data != "first\nsecond" {
		t.Fatalf("first event = %#v", first)
	}

	second, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != "stream-1" || second.Event != "message" || second.Data != "third" {
		t.Fatalf("second event = %#v", second)
	}
}

func TestReaderControlFieldsUpdateStateWithoutDispatchingEvents(t *testing.T) {
	input := "id: previous\nretry: 1500\nevent: ignored\n\n" +
		"data: one\n\n" +
		"id:\nretry: invalid\n\n" +
		"data: two\n\n"
	reader := MustNewReader(strings.NewReader(input))

	first, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if first.Data != "one" || first.ID != "previous" || first.Event != "message" {
		t.Fatalf("first dispatched event = %#v", first)
	}
	if reader.LastEventID() != "previous" {
		t.Fatalf("LastEventID() = %q, want previous", reader.LastEventID())
	}

	second, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if second.Data != "two" || second.ID != "" || second.Event != "message" {
		t.Fatalf("second dispatched event = %#v", second)
	}
	if reader.LastEventID() != "" {
		t.Fatalf("LastEventID() = %q, want empty", reader.LastEventID())
	}
}

func TestReaderDispatchesEmptyDataFields(t *testing.T) {
	reader := MustNewReader(strings.NewReader("data\n\ndata:\n\n"))
	for index := range 2 {
		event, err := reader.Read()
		if err != nil {
			t.Fatalf("event %d: %v", index, err)
		}
		if event.Data != "" || event.Event != "message" || event.IsEmpty() {
			t.Fatalf("event %d = %#v, want dispatched empty-data message", index, event)
		}
	}
}

func TestReaderDiscardsIncompleteEventAtEOF(t *testing.T) {
	reader := MustNewReader(strings.NewReader("data: incomplete"))
	event, err := reader.Read()
	if event != nil || !errors.Is(err, io.EOF) {
		t.Fatalf("Read() = (%#v, %v), want (nil, io.EOF)", event, err)
	}
}

func TestReaderDecodesInvalidUTF8WithReplacement(t *testing.T) {
	input := append([]byte("data: "), 0xff)
	input = append(input, '\n', '\n')
	reader := MustNewReader(bytes.NewReader(input))
	event, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if event.Data != "\uFFFD" {
		t.Fatalf("Read() data = %q, want UTF-8 replacement character", event.Data)
	}
}

func TestWriterEncodesEmptyDataEvent(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := MustNewWriter(recorder)
	if err := writer.WriteData(""); err != nil {
		t.Fatal(err)
	}
	if got := recorder.Body.String(); got != "data: \n\n" {
		t.Fatalf("WriteData(\"\") = %q, want empty data field", got)
	}

	formatted, err := FormatEvent(&Event{})
	if err != nil {
		t.Fatal(err)
	}
	if formatted != "data: \n\n" {
		t.Fatalf("FormatEvent(empty) = %q, want empty data field", formatted)
	}
}

func TestReaderLimitFailureIsTerminal(t *testing.T) {
	source := strings.NewReader("data: oversized\n\ndata: must-not-be-read\n\n")
	reader := MustNewReaderWithOptions(source, WithMaxTotalBytes(8))

	if _, err := reader.Read(); !errors.Is(err, ErrMaxBytesExceeded) {
		t.Fatalf("first Read() error = %v, want ErrMaxBytesExceeded", err)
	}
	buffered := reader.reader.Buffered()
	if _, err := reader.Read(); !errors.Is(err, ErrMaxBytesExceeded) {
		t.Fatalf("second Read() error = %v, want ErrMaxBytesExceeded", err)
	}
	if reader.reader.Buffered() != buffered {
		t.Fatalf("second Read() consumed %d buffered bytes after terminal failure", buffered-reader.reader.Buffered())
	}
}

func TestFormatEventPreservesEmptyIDClear(t *testing.T) {
	reader := MustNewReader(strings.NewReader("id:\ndata: payload\n\n"))
	event, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}

	formatted, err := FormatEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	if formatted != "id: \nevent: message\ndata: payload\n\n" {
		t.Fatalf("FormatEvent() = %q, want an explicit empty id field", formatted)
	}
}
