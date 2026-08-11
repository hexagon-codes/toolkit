package json

import (
	"bytes"
	stdjson "encoding/json"
	"errors"
	"strings"
	"testing"
)

type auditNilReader struct{}

func (*auditNilReader) Read([]byte) (int, error) {
	panic("typed nil reader was invoked")
}

type auditNilWriter struct{}

func (*auditNilWriter) Write([]byte) (int, error) {
	panic("typed nil writer was invoked")
}

func TestToMapPreservesIntegerPrecision(t *testing.T) {
	result, err := ToMap(`{"id":9007199254740993}`)
	if err != nil {
		t.Fatalf("ToMap() error: %v", err)
	}
	number, ok := result["id"].(stdjson.Number)
	if !ok || number.String() != "9007199254740993" {
		t.Fatalf("id = %T(%v), want exact json.Number", result["id"], result["id"])
	}
}

func TestUnmarshalRejectsUnknownFields(t *testing.T) {
	var target struct {
		Known int `json:"known"`
	}
	err := Unmarshal(`{"known":1,"unknown":2}`, &target)
	if !errors.Is(err, ErrInvalidJSON) {
		t.Fatal("unknown field was accepted")
	}
	if target.Known != 0 {
		t.Fatalf("failed decode partially mutated target: %+v", target)
	}
}

func TestUnmarshalRejectsOversizedAndExcessivelyNestedDocuments(t *testing.T) {
	oversized := `{"value":"` + strings.Repeat("x", 8<<20) + `"}`
	var target map[string]any
	if err := Unmarshal(oversized, &target); !errors.Is(err, ErrDocumentTooLarge) {
		t.Fatalf("oversized JSON error = %v, want ErrDocumentTooLarge", err)
	}

	deep := strings.Repeat("[", 101) + strings.Repeat("]", 101)
	if err := Unmarshal(deep, &target); !errors.Is(err, ErrNestingTooDeep) {
		t.Fatalf("deep JSON error = %v, want ErrNestingTooDeep", err)
	}
}

func TestNDJSONDecoderClassifiesSyntaxErrors(t *testing.T) {
	decoder := NewNDJSONDecoder(strings.NewReader("{invalid}\n"))
	var target map[string]any
	err := decoder.Decode(&target)
	if !errors.Is(err, ErrInvalidJSON) {
		t.Fatalf("Decode() error = %v, want ErrInvalidJSON", err)
	}
	if decoder.Err() == nil || !errors.Is(decoder.Err(), ErrInvalidJSON) {
		t.Fatalf("Err() = %v, want retained ErrInvalidJSON", decoder.Err())
	}
	var syntaxError *stdjson.SyntaxError
	if !errors.As(err, &syntaxError) {
		t.Fatalf("Decode() lost syntax error chain: %v", err)
	}
}

func TestDecodersRejectNilReadersWithoutPanicking(t *testing.T) {
	var typedNil *auditNilReader
	tests := []struct {
		name   string
		decode func(any) error
	}{
		{name: "stream", decode: NewStreamDecoder(nil).Decode},
		{name: "ndjson", decode: NewNDJSONDecoder(nil).Decode},
		{name: "sse", decode: NewSSEJSONDecoder(nil).Decode},
		{name: "typed nil stream", decode: NewStreamDecoder(typedNil).Decode},
		{name: "typed nil ndjson", decode: NewNDJSONDecoder(typedNil).Decode},
		{name: "typed nil sse", decode: NewSSEJSONDecoder(typedNil).Decode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("Decode() panicked: %v", recovered)
				}
			}()
			var target any
			if err := tt.decode(&target); err == nil {
				t.Fatal("Decode() accepted a nil reader")
			}
		})
	}
}

func TestEncodersRejectTypedNilWritersWithoutPanicking(t *testing.T) {
	var writer *auditNilWriter
	for name, encode := range map[string]func(any) error{
		"stream": NewStreamEncoder(writer).Encode,
		"ndjson": NewNDJSONEncoder(writer).Encode,
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("Encode() panicked: %v", recovered)
				}
			}()
			if err := encode(map[string]int{"id": 1}); !errors.Is(err, ErrInvalidWriter) {
				t.Fatalf("Encode() error = %v, want ErrInvalidWriter", err)
			}
		})
	}
}

func TestStreamRecordLimits(t *testing.T) {
	stream := NewStreamDecoderWithSize(strings.NewReader(`{"value":"too large"}`), 8)
	var target map[string]any
	if err := stream.Decode(&target); !errors.Is(err, ErrDocumentTooLarge) {
		t.Fatalf("stream Decode() error = %v, want ErrDocumentTooLarge", err)
	}

	ndjson := NewNDJSONDecoderWithSize(strings.NewReader(`{"value":"too large"}`+"\n"), 8)
	if err := ndjson.Decode(&target); !errors.Is(err, ErrRecordTooLarge) {
		t.Fatalf("NDJSON Decode() error = %v, want ErrRecordTooLarge", err)
	}
}

func TestSSEJSONDecoderJoinsMultilineDataEvent(t *testing.T) {
	input := "data: {\"id\":\ndata: 1}\n\n"
	decoder := NewSSEJSONDecoder(strings.NewReader(input))
	var target struct {
		ID int `json:"id"`
	}
	if err := decoder.Decode(&target); err != nil {
		t.Fatalf("Decode() error: %v", err)
	}
	if target.ID != 1 {
		t.Fatalf("decoded ID = %d, want 1", target.ID)
	}
}

func TestSSEJSONDecoderTreatsDataFieldWithoutColonAsEmpty(t *testing.T) {
	input := "data\n\ndata: {\"id\":1}\n\n"
	decoder := NewSSEJSONDecoder(strings.NewReader(input))
	var target struct {
		ID int `json:"id"`
	}
	if err := decoder.Decode(&target); err != nil {
		t.Fatalf("Decode() error: %v", err)
	}
	if target.ID != 1 {
		t.Fatalf("decoded ID = %d, want 1", target.ID)
	}
}

func TestNDJSONDecoderAcceptsRecordAtExactSizeLimit(t *testing.T) {
	decoder := NewNDJSONDecoderWithSize(strings.NewReader("0\n"), 1)
	var target int
	if err := decoder.Decode(&target); err != nil {
		t.Fatalf("Decode() error: %v", err)
	}
	if target != 0 {
		t.Fatalf("decoded value = %d, want 0", target)
	}
}

func TestNDJSONEncoderUsesSingleRecordWrite(t *testing.T) {
	var output bytes.Buffer
	encoder := NewNDJSONEncoder(&output)
	if err := encoder.Encode(map[string]int{"id": 1}); err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
	if got := output.String(); got != "{\"id\":1}\n" {
		t.Fatalf("encoded record = %q", got)
	}
}

func TestNDJSONDecoderRejectsInvalidSizeWithoutPanicking(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("constructor panicked: %v", recovered)
		}
	}()
	decoder := NewNDJSONDecoderWithSize(strings.NewReader(`{"id":1}`), -1)
	var target any
	if err := decoder.Decode(&target); err == nil {
		t.Fatal("decoder accepted a negative maximum record size")
	}
}
