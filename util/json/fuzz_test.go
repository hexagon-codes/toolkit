package json

import (
	"bytes"
	"testing"
)

func FuzzUnmarshalBytesNeverPanics(f *testing.F) {
	f.Add([]byte(`{"id":1}`))
	f.Add([]byte(`[[[]]]`))
	f.Add([]byte(`{invalid}`))
	f.Fuzz(func(t *testing.T, input []byte) {
		var target any
		_ = UnmarshalBytes(input, &target)
	})
}

func FuzzNDJSONDecoderNeverPanics(f *testing.F) {
	f.Add([]byte("{\"id\":1}\n"), uint16(64))
	f.Add([]byte("invalid\n"), uint16(1))
	f.Fuzz(func(t *testing.T, input []byte, rawSize uint16) {
		size := int(rawSize%4096) + 1
		decoder := NewNDJSONDecoderWithSize(bytes.NewReader(input), size)
		var target any
		_ = decoder.Decode(&target)
	})
}
