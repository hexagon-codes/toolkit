package json

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestStreamDecoder(t *testing.T) {
	input := `{"id":1}{"id":2}{"id":3}`
	decoder := NewStreamDecoder(strings.NewReader(input))

	type Item struct {
		ID int `json:"id"`
	}

	var items []Item
	for decoder.More() {
		var item Item
		if err := decoder.Decode(&item); err != nil {
			t.Fatalf("decode error: %v", err)
		}
		items = append(items, item)
	}

	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
	if items[0].ID != 1 || items[1].ID != 2 || items[2].ID != 3 {
		t.Errorf("unexpected items: %v", items)
	}
}

func TestStreamDecoder_Close(t *testing.T) {
	decoder := NewStreamDecoder(strings.NewReader(`{"id":1}`))
	decoder.Close()

	var v any
	err := decoder.Decode(&v)
	if err != ErrStreamClosed {
		t.Errorf("expected ErrStreamClosed, got %v", err)
	}
}

func TestNDJSONDecoder(t *testing.T) {
	input := `{"id":1}
{"id":2}
{"id":3}
`
	decoder := NewNDJSONDecoder(strings.NewReader(input))

	type Item struct {
		ID int `json:"id"`
	}

	var items []Item
	for {
		var item Item
		err := decoder.Decode(&item)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decode error: %v", err)
		}
		items = append(items, item)
	}

	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
}

func TestNDJSONDecoder_SkipEmptyLines(t *testing.T) {
	input := `{"id":1}

{"id":2}

{"id":3}
`
	decoder := NewNDJSONDecoder(strings.NewReader(input))

	type Item struct {
		ID int `json:"id"`
	}

	count := 0
	for {
		var item Item
		err := decoder.Decode(&item)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decode error: %v", err)
		}
		count++
	}

	if count != 3 {
		t.Errorf("expected 3 items, got %d", count)
	}
}

func TestNDJSONDecoder_Close(t *testing.T) {
	decoder := NewNDJSONDecoder(strings.NewReader(`{"id":1}`))
	decoder.Close()

	var v any
	err := decoder.Decode(&v)
	if err != ErrStreamClosed {
		t.Errorf("expected ErrStreamClosed, got %v", err)
	}
}

func TestStreamEncoder(t *testing.T) {
	var buf bytes.Buffer
	encoder := NewStreamEncoder(&buf)

	type Item struct {
		ID int `json:"id"`
	}

	items := []Item{{ID: 1}, {ID: 2}, {ID: 3}}
	for _, item := range items {
		if err := encoder.Encode(item); err != nil {
			t.Fatalf("encode error: %v", err)
		}
	}

	expected := `{"id":1}
{"id":2}
{"id":3}
`
	if buf.String() != expected {
		t.Errorf("expected %q, got %q", expected, buf.String())
	}
}

func TestStreamEncoder_Close(t *testing.T) {
	var buf bytes.Buffer
	encoder := NewStreamEncoder(&buf)
	encoder.Close()

	err := encoder.Encode(map[string]int{"id": 1})
	if err != ErrStreamClosed {
		t.Errorf("expected ErrStreamClosed, got %v", err)
	}
}

func TestNDJSONEncoder(t *testing.T) {
	var buf bytes.Buffer
	encoder := NewNDJSONEncoder(&buf)

	type Item struct {
		ID int `json:"id"`
	}

	items := []Item{{ID: 1}, {ID: 2}}
	for _, item := range items {
		if err := encoder.Encode(item); err != nil {
			t.Fatalf("encode error: %v", err)
		}
	}

	expected := `{"id":1}
{"id":2}
`
	if buf.String() != expected {
		t.Errorf("expected %q, got %q", expected, buf.String())
	}
}

func TestNDJSONEncoder_Close(t *testing.T) {
	var buf bytes.Buffer
	encoder := NewNDJSONEncoder(&buf)
	encoder.Close()

	err := encoder.Encode(map[string]int{"id": 1})
	if err != ErrStreamClosed {
		t.Errorf("expected ErrStreamClosed, got %v", err)
	}
}

func TestSSEJSONDecoder(t *testing.T) {
	input := `data: {"id":1}

data: {"id":2}

data: [DONE]
`
	decoder := NewSSEJSONDecoder(strings.NewReader(input))

	type Item struct {
		ID int `json:"id"`
	}

	var items []Item
	for {
		var item Item
		err := decoder.Decode(&item)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decode error: %v", err)
		}
		items = append(items, item)
	}

	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
	if items[0].ID != 1 || items[1].ID != 2 {
		t.Errorf("unexpected items: %v", items)
	}
}

func TestSSEJSONDecoder_SkipCommentsAndOtherFields(t *testing.T) {
	input := `: this is a comment
event: message
id: 123
data: {"value":"test"}

`
	decoder := NewSSEJSONDecoder(strings.NewReader(input))

	type Item struct {
		Value string `json:"value"`
	}

	var item Item
	err := decoder.Decode(&item)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if item.Value != "test" {
		t.Errorf("expected 'test', got '%s'", item.Value)
	}
}

func TestSSEJSONDecoder_CustomDone(t *testing.T) {
	input := `data: {"id":1}

data: END
`
	decoder := NewSSEJSONDecoderWithDone(strings.NewReader(input), "END")

	type Item struct {
		ID int `json:"id"`
	}

	var item Item
	err := decoder.Decode(&item)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if item.ID != 1 {
		t.Errorf("expected id 1, got %d", item.ID)
	}

	// 下一个应该是 EOF
	err = decoder.Decode(&item)
	if err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
}

func TestSSEJSONDecoder_Close(t *testing.T) {
	decoder := NewSSEJSONDecoder(strings.NewReader(`data: {"id":1}`))
	decoder.Close()

	var v any
	err := decoder.Decode(&v)
	if err != ErrStreamClosed {
		t.Errorf("expected ErrStreamClosed, got %v", err)
	}
}

func TestDecodeStream(t *testing.T) {
	// 流式 JSON 对象（非数组）
	input := `{"id":1}{"id":2}{"id":3}`

	type Item struct {
		ID int `json:"id"`
	}

	items, err := DecodeStream[Item](strings.NewReader(input))
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
}

func TestDecodeNDJSON(t *testing.T) {
	input := `{"id":1}
{"id":2}
{"id":3}
`
	type Item struct {
		ID int `json:"id"`
	}

	items, err := DecodeNDJSON[Item](strings.NewReader(input))
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
}

func TestDecodeSSEJSON(t *testing.T) {
	input := `data: {"id":1}

data: {"id":2}

data: [DONE]
`
	type Item struct {
		ID int `json:"id"`
	}

	items, err := DecodeSSEJSON[Item](strings.NewReader(input))
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestEncodeNDJSON(t *testing.T) {
	type Item struct {
		ID int `json:"id"`
	}

	items := []Item{{ID: 1}, {ID: 2}, {ID: 3}}
	data, err := EncodeNDJSON(items)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}

	expected := `{"id":1}
{"id":2}
{"id":3}
`
	if string(data) != expected {
		t.Errorf("expected %q, got %q", expected, string(data))
	}
}

func TestStreamIterator(t *testing.T) {
	// 流式 JSON 对象（非数组）
	input := `{"id":1}{"id":2}{"id":3}`

	type Item struct {
		ID int `json:"id"`
	}

	iter := NewStreamIterator[Item](strings.NewReader(input))

	var items []Item
	for iter.Next() {
		items = append(items, iter.Value())
	}

	if iter.Err() != nil {
		t.Errorf("unexpected error: %v", iter.Err())
	}

	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
}

func TestNDJSONIterator(t *testing.T) {
	input := `{"id":1}
{"id":2}
{"id":3}
`
	type Item struct {
		ID int `json:"id"`
	}

	iter := NewNDJSONIterator[Item](strings.NewReader(input))

	var items []Item
	for iter.Next() {
		items = append(items, iter.Value())
	}

	if iter.Err() != nil {
		t.Errorf("unexpected error: %v", iter.Err())
	}

	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
}

func TestSSEJSONIterator(t *testing.T) {
	input := `data: {"id":1}

data: {"id":2}

data: [DONE]
`
	type Item struct {
		ID int `json:"id"`
	}

	iter := NewSSEJSONIterator[Item](strings.NewReader(input))

	var items []Item
	for iter.Next() {
		items = append(items, iter.Value())
	}

	if iter.Err() != nil {
		t.Errorf("unexpected error: %v", iter.Err())
	}

	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestNewStreamDecoderWithSize(t *testing.T) {
	input := `{"id":1}{"id":2}`
	decoder := NewStreamDecoderWithSize(strings.NewReader(input), 1024)

	type Item struct {
		ID int `json:"id"`
	}

	var item Item
	if err := decoder.Decode(&item); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if item.ID != 1 {
		t.Errorf("expected id 1, got %d", item.ID)
	}
}

func TestNewNDJSONDecoderWithSize(t *testing.T) {
	input := `{"id":1}
{"id":2}
`
	decoder := NewNDJSONDecoderWithSize(strings.NewReader(input), 1024)

	type Item struct {
		ID int `json:"id"`
	}

	var item Item
	if err := decoder.Decode(&item); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if item.ID != 1 {
		t.Errorf("expected id 1, got %d", item.ID)
	}
}

func TestStreamEncoder_SetIndent(t *testing.T) {
	var buf bytes.Buffer
	encoder := NewStreamEncoder(&buf)
	encoder.SetIndent("", "  ")

	type Item struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	if err := encoder.Encode(Item{ID: 1, Name: "test"}); err != nil {
		t.Fatalf("encode error: %v", err)
	}

	expected := `{
  "id": 1,
  "name": "test"
}
`
	if buf.String() != expected {
		t.Errorf("expected %q, got %q", expected, buf.String())
	}
}

func TestStreamEncoder_SetEscapeHTML(t *testing.T) {
	var buf bytes.Buffer
	encoder := NewStreamEncoder(&buf)
	encoder.SetEscapeHTML(false)

	data := map[string]string{"html": "<script>"}
	if err := encoder.Encode(data); err != nil {
		t.Fatalf("encode error: %v", err)
	}

	// 不转义 HTML 时，< > 应该保持原样
	if !strings.Contains(buf.String(), "<script>") {
		t.Errorf("expected unescaped HTML, got %s", buf.String())
	}
}
